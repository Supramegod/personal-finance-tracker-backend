package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"personal-finance-tracker/internal/repository"
)

type InsightFacts struct {
	TotalIncome      float64            `json:"total_income"`
	TotalExpense     float64            `json:"total_expense"`
	Net              float64            `json:"net"`
	SavingsRate      float64            `json:"savings_rate_percent"`
	ExpenseChange    *float64           `json:"expense_change_percent,omitempty"`
	TransactionCount int                `json:"transaction_count"`
	TopCategories    []CategorySpending `json:"top_expense_categories"`
}

type CategorySpending struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Share  float64 `json:"share_percent"`
}
type InsightRecommendation struct {
	Title    string `json:"title"`
	Action   string `json:"action"`
	Priority string `json:"priority"`
}
type InsightAnalysis struct {
	Headline        string                  `json:"headline"`
	Summary         string                  `json:"summary"`
	HealthStatus    string                  `json:"health_status"`
	KeyFindings     []string                `json:"key_findings"`
	Recommendations []InsightRecommendation `json:"recommendations"`
	Cautions        []string                `json:"cautions"`
}

type InsightResponse struct {
	Status      string           `json:"status"`
	Period      string           `json:"period"`
	Facts       InsightFacts     `json:"facts"`
	Analysis    *InsightAnalysis `json:"analysis,omitempty"`
	GeneratedAt *time.Time       `json:"generated_at,omitempty"`
	Model       string           `json:"model"`
	IsStale     bool             `json:"is_stale"`
	Error       *string          `json:"error,omitempty"`
}

// Tipe di bawah hanya dikirim ke model, tidak pernah disimpan di kolom facts
// maupun dikembalikan ke klien. InsightFacts sengaja dibiarkan utuh supaya
// InsightResponse, panel React, model Flutter, dan swagger tidak ikut berubah.
type CategoryChange struct {
	Name           string   `json:"name"`
	Amount         float64  `json:"amount"`
	PreviousAmount float64  `json:"previous_amount"`
	ChangePercent  *float64 `json:"change_percent"`
	IsNew          bool     `json:"is_new"`
}

type WeeklyFlow struct {
	Week    int     `json:"week"`
	Expense float64 `json:"expense"`
	Income  float64 `json:"income"`
}

type LargestExpense struct {
	Date     string  `json:"date"`
	Category string  `json:"category"`
	Amount   float64 `json:"amount"`
}

type insightInput struct {
	Period                     string           `json:"period"`
	PeriodLabel                string           `json:"period_label"`
	Currency                   string           `json:"currency"`
	DaysInPeriod               int              `json:"days_in_period"`
	HasPreviousMonth           bool             `json:"has_previous_month"`
	PreviousTotalIncome        *float64         `json:"previous_total_income,omitempty"`
	PreviousTotalExpense       *float64         `json:"previous_total_expense,omitempty"`
	Facts                      InsightFacts     `json:"facts"`
	CategoryChanges            []CategoryChange `json:"category_changes"`
	Weekly                     []WeeklyFlow     `json:"weekly"`
	LargestExpenses            []LargestExpense `json:"largest_expenses"`
	ExpenseTransactionCount    int              `json:"expense_transaction_count"`
	IncomeTransactionCount     int              `json:"income_transaction_count"`
	ActiveDays                 int              `json:"active_days"`
	WeekendExpenseSharePercent float64          `json:"weekend_expense_share_percent"`
}

type AIInsightService struct {
	repo                         *repository.AIInsightRepository
	client                       *http.Client
	apiKey, model, promptVersion string
	enabled                      bool
}

func NewAIInsightService(repo *repository.AIInsightRepository, apiKey, model, promptVersion string, timeout time.Duration, enabled bool) *AIInsightService {
	return &AIInsightService{repo: repo, client: &http.Client{Timeout: timeout}, apiKey: apiKey, model: model, promptVersion: effectivePromptVersion(promptVersion), enabled: enabled}
}

// PromptVersion mengembalikan versi efektif — label dari env digabung sidik
// jari isi prompt. Dicatat sekali saat startup supaya deploy yang mengubah
// prompt terlihat di log tanpa perlu query database.
func (s *AIInsightService) PromptVersion() string { return s.promptVersion }

func (s *AIInsightService) GetConsent(groupID, userID string) (*repository.AIConsent, error) {
	consent, err := s.repo.GetConsent(groupID, userID)
	if consent != nil {
		consent.Available = s.enabled && s.apiKey != ""
	}
	return consent, err
}
func (s *AIInsightService) SetConsent(groupID, userID string, enabled bool) (*repository.AIConsent, error) {
	if enabled && (!s.enabled || s.apiKey == "") {
		return nil, errors.New("AI insights are not configured")
	}
	consent, err := s.repo.SetConsent(groupID, userID, enabled)
	if consent != nil {
		consent.Available = s.enabled && s.apiKey != ""
	}
	if err == nil && enabled && s.enabled && s.apiKey != "" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			_ = s.Generate(ctx, groupID, previousMonth(time.Now()))
		}()
	}
	return consent, err
}

// regenerateCooldown menahan tombol "buat ulang" supaya satu klik beruntun
// tidak membakar kuota Gemini. Satu regenerasi = satu panggilan berbayar.
const regenerateCooldown = 5 * time.Minute

var ErrRegenerateFutureMonth = errors.New("month is not finished yet")

// Regenerate memaksa satu bulan dibuat ulang atas permintaan owner kelompok.
//
// Kembali segera setelah baris disiapkan; pembuatannya berjalan di latar
// seperti jalur consent, dan klien memantau lewat polling status yang sudah
// ada. Batas waktunya memakai perGroupTimeout() yang sama dengan scheduler,
// jadi satu permintaan manual tidak bisa menggantung lebih lama daripada
// sapuan terjadwal.
func (s *AIInsightService) Regenerate(groupID, userID string, month time.Time) error {
	if !s.enabled || s.apiKey == "" {
		return errors.New("AI insights are not configured")
	}
	month = normalizeMonth(month)
	// Bulan berjalan datanya belum lengkap, dan bulan depan belum ada apa-apa.
	if !month.Before(normalizeMonth(time.Now())) {
		return ErrRegenerateFutureMonth
	}
	if err := s.repo.PrepareRegenerate(groupID, userID, month, regenerateCooldown); err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.perGroupTimeout())
		defer cancel()
		_ = s.Generate(ctx, groupID, month)
	}()
	return nil
}

func (s *AIInsightService) Get(groupID string, month time.Time) (*InsightResponse, error) {
	i, err := s.repo.Get(groupID, normalizeMonth(month))
	if err != nil {
		return nil, err
	}
	return insightResponse(i)
}
func (s *AIInsightService) Latest(groupID string) (*InsightResponse, error) {
	i, err := s.repo.Latest(groupID)
	if err != nil {
		return nil, err
	}
	return insightResponse(i)
}

func (s *AIInsightService) GeneratePreviousMonthForEnabled(ctx context.Context, now time.Time) error {
	if !s.enabled || s.apiKey == "" {
		return nil
	}
	groups, err := s.repo.EnabledGroups()
	if err != nil {
		return err
	}
	month := previousMonth(now)
	for _, groupID := range groups {
		// Setiap grup memperoleh batas waktunya sendiri. Sebelumnya satu
		// context dipakai bersama untuk seluruh sapuan, sehingga begitu
		// batas itu habis SEMUA grup yang belum sempat diproses langsung
		// ditandai gagal — grup yang lambat menjatuhkan grup sesudahnya.
		groupCtx, cancel := context.WithTimeout(ctx, s.perGroupTimeout())
		err := s.Generate(groupCtx, groupID, month)
		cancel()
		if err != nil { /* lanjutkan grup berikutnya */
			continue
		}
	}
	return nil
}

// perGroupTimeout membatasi satu grup dalam sapuan terjadwal, DITURUNKAN dari
// AI_TIMEOUT alih-alih ditulis sebagai angka tetap.
//
// Sebelumnya nilainya konstan 3 menit, dihitung tangan dari asumsi AI_TIMEOUT
// 30 detik. Begitu AI_TIMEOUT dinaikkan melewati ~55 detik, tiga percobaan
// tidak lagi muat dalam 3 menit: scheduler memotong grup di tengah percobaan
// kedua lalu menandainya gagal — bukan karena Gemini bermasalah, melainkan
// karena dua konstanta yang tidak lagi sepakat. Menurunkannya membuat
// keduanya mustahil berselisih.
//
// backoffTotal adalah jumlah jeda antar percobaan (1s + 2s untuk 3 percobaan);
// marginnya menutup pembangunan payload dan penulisan hasil ke database.
func (s *AIInsightService) perGroupTimeout() time.Duration {
	const backoffTotal, margin = 3 * time.Second, 30 * time.Second
	return time.Duration(geminiAttempts)*s.client.Timeout + backoffTotal + margin
}

func (s *AIInsightService) Generate(ctx context.Context, groupID string, month time.Time) error {
	if !s.enabled || s.apiKey == "" {
		return errors.New("AI insights are not configured")
	}
	month = normalizeMonth(month)
	next := month.AddDate(0, 1, 0)
	items, err := s.repo.Transactions(groupID, month, next)
	if err != nil {
		return err
	}
	previous, err := s.repo.Transactions(groupID, month.AddDate(0, -1, 0), month)
	if err != nil {
		return err
	}
	facts := buildFacts(items, previous)
	factsJSON, _ := json.Marshal(facts)
	// Hash TETAP dihitung dari transaksi mentah meski yang dikirim ke model
	// sudah berupa agregat. Tampilan turunan adalah fungsi deterministik dari
	// transaksi, jadi mengedit satu transaksi yang kebetulan tidak menggeser
	// bucket mana pun harus tetap memicu regenerasi.
	source, _ := json.Marshal(struct {
		Transactions []repository.InsightTransaction `json:"transactions"`
		Facts        InsightFacts                    `json:"facts"`
	}{items, facts})
	h := sha256.Sum256(append(source, []byte(s.promptVersion)...))
	hash := hex.EncodeToString(h[:])
	claimed, err := s.repo.Claim(groupID, month, factsJSON, s.model, s.promptVersion, hash)
	if err != nil || !claimed {
		return err
	}

	analysis, err := s.callGemini(ctx, month, facts, items, previous)
	if err != nil {
		_ = s.repo.Fail(groupID, month, err.Error())
		return err
	}
	data, _ := json.Marshal(analysis)
	return s.repo.Complete(groupID, month, data)
}

func buildFacts(items, previous []repository.InsightTransaction) InsightFacts {
	var f InsightFacts
	categories := map[string]float64{}
	var previousExpense float64
	for _, item := range items {
		f.TransactionCount++
		if item.Type == "income" {
			f.TotalIncome += item.Amount
		} else {
			f.TotalExpense += item.Amount
			categories[item.Category] += item.Amount
		}
	}
	for _, item := range previous {
		if item.Type == "expense" {
			previousExpense += item.Amount
		}
	}
	f.Net = f.TotalIncome - f.TotalExpense
	if f.TotalIncome > 0 {
		f.SavingsRate = f.Net / f.TotalIncome * 100
	}
	if previousExpense > 0 {
		change := (f.TotalExpense - previousExpense) / previousExpense * 100
		f.ExpenseChange = &change
	}
	for name, amount := range categories {
		share := 0.0
		if f.TotalExpense > 0 {
			share = amount / f.TotalExpense * 100
		}
		f.TopCategories = append(f.TopCategories, CategorySpending{name, amount, share})
	}
	sort.Slice(f.TopCategories, func(i, j int) bool { return f.TopCategories[i].Amount > f.TopCategories[j].Amount })
	if len(f.TopCategories) > 5 {
		f.TopCategories = f.TopCategories[:5]
	}
	return f
}

// Batas keras representasi yang dikirim ke model. Sebelumnya seluruh transaksi
// bulan itu dikirim mentah tanpa LIMIT: satu grup yang mengimpor riwayat
// setahun ke satu bulan bisa menghabiskan puluhan ribu token, dan daftar
// ratusan baris mengundang model menghitung sendiri lalu berselisih dengan
// facts. Dengan batas ini, grup 5.000 transaksi berbiaya sama dengan grup 50.
const (
	maxCategoryChanges = 12
	maxLargestExpenses = 10
	maxWeeklyBuckets   = 5
)

func buildInsightInput(month time.Time, facts InsightFacts, items, previous []repository.InsightTransaction) insightInput {
	next := month.AddDate(0, 1, 0)
	in := insightInput{
		Period:           month.Format("2006-01"),
		PeriodLabel:      indonesianMonthLabel(month),
		Currency:         "IDR",
		DaysInPeriod:     int(next.Sub(month).Hours() / 24),
		HasPreviousMonth: len(previous) > 0,
		Facts:            facts,
		CategoryChanges:  []CategoryChange{},
		Weekly:           []WeeklyFlow{},
		LargestExpenses:  []LargestExpense{},
	}

	current := map[string]float64{}
	weeks := map[int]*WeeklyFlow{}
	days := map[string]struct{}{}
	largest := []LargestExpense{}
	var weekendExpense float64

	for _, item := range items {
		days[item.Date] = struct{}{}
		flow := weeks[weekOfMonth(item.Date)]
		if flow == nil {
			flow = &WeeklyFlow{Week: weekOfMonth(item.Date)}
			weeks[flow.Week] = flow
		}
		if item.Type == "income" {
			in.IncomeTransactionCount++
			flow.Income += item.Amount
			continue
		}
		in.ExpenseTransactionCount++
		flow.Expense += item.Amount
		current[item.Category] += item.Amount
		largest = append(largest, LargestExpense{item.Date, item.Category, item.Amount})
		if isWeekend(item.Date) {
			weekendExpense += item.Amount
		}
	}
	in.ActiveDays = len(days)
	if facts.TotalExpense > 0 {
		in.WeekendExpenseSharePercent = round1(weekendExpense / facts.TotalExpense * 100)
	}

	if len(previous) > 0 {
		var previousIncome, previousExpense float64
		for _, item := range previous {
			if item.Type == "income" {
				previousIncome += item.Amount
			} else {
				previousExpense += item.Amount
			}
		}
		in.PreviousTotalIncome = &previousIncome
		in.PreviousTotalExpense = &previousExpense
	}

	in.CategoryChanges = buildCategoryChanges(current, previous)

	for _, flow := range weeks {
		in.Weekly = append(in.Weekly, *flow)
	}
	sort.Slice(in.Weekly, func(i, j int) bool { return in.Weekly[i].Week < in.Weekly[j].Week })

	sort.Slice(largest, func(i, j int) bool { return largest[i].Amount > largest[j].Amount })
	if len(largest) > maxLargestExpenses {
		largest = largest[:maxLargestExpenses]
	}
	in.LargestExpenses = largest

	return in
}

// buildCategoryChanges mengurutkan berdasarkan BESAR PERGERAKAN, bukan besar
// nominal. Daftar terbesar-menurut-nominal sudah ada sebagai
// top_expense_categories; yang berguna dari daftar kedua justru kategori yang
// berubah, karena itu yang layak ditulis satu kalimat.
func buildCategoryChanges(current map[string]float64, previous []repository.InsightTransaction) []CategoryChange {
	before := map[string]float64{}
	for _, item := range previous {
		if item.Type == "expense" {
			before[item.Category] += item.Amount
		}
	}

	names := map[string]struct{}{}
	for name := range current {
		names[name] = struct{}{}
	}
	for name := range before {
		names[name] = struct{}{}
	}

	changes := []CategoryChange{}
	for name := range names {
		change := CategoryChange{Name: name, Amount: current[name], PreviousAmount: before[name]}
		if before[name] > 0 {
			percent := round1((current[name] - before[name]) / before[name] * 100)
			change.ChangePercent = &percent
		} else {
			// change_percent WAJIB null, bukan 0 dan bukan Inf. Memancarkan 0
			// untuk kategori yang baru muncul adalah cara mendapatkan
			// "Listrik naik 0%" di produksi.
			change.IsNew = true
		}
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool {
		return math.Abs(changes[i].Amount-changes[i].PreviousAmount) > math.Abs(changes[j].Amount-changes[j].PreviousAmount)
	})
	if len(changes) > maxCategoryChanges {
		changes = changes[:maxCategoryChanges]
	}
	return changes
}

func weekOfMonth(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 1
	}
	if week := (t.Day()-1)/7 + 1; week <= maxWeeklyBuckets {
		return week
	}
	return maxWeeklyBuckets
}

func isWeekend(date string) bool {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// round1 membulatkan ke satu desimal supaya model tidak menerima
// 34.13793103448276 lalu menuliskannya apa adanya. Prompt meminta persentase
// satu desimal; angka masukannya harus sudah sesuai bentuk itu agar pemeriksa
// grounding di eval bisa mencocokkannya kembali.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

const (
	geminiAttempts   = 3
	baseTemperature  = 0.2
	temperatureStep  = 0.2
	geminiEndpointFm = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"
)

func buildRequestBody(input []byte, temperature float64, thinkingBudget int) map[string]any {
	return map[string]any{
		"systemInstruction": map[string]any{
			"parts": []any{map[string]any{"text": systemPrompt}},
		},
		"contents": []any{map[string]any{
			"role": "user",
			"parts": []any{map[string]any{
				"text": "Analisis data berikut, lalu isi seluruh field sesuai kontrak.\n\n<data>\n" + string(input) + "\n</data>",
			}},
		}},
		"generationConfig": map[string]any{
			"temperature":      temperature,
			"maxOutputTokens":  maxOutputTokens,
			"responseMimeType": "application/json",
			"responseSchema":   analysisSchema(),
			"thinkingConfig": map[string]any{
				"thinkingBudget":  thinkingBudget,
				"includeThoughts": false,
			},
		},
	}
}

func (s *AIInsightService) callGemini(ctx context.Context, month time.Time, facts InsightFacts, items, previous []repository.InsightTransaction) (*InsightAnalysis, error) {
	input, _ := json.Marshal(buildInsightInput(month, facts, items, previous))
	endpoint := fmt.Sprintf(geminiEndpointFm, url.PathEscape(s.model))

	temperature, thinkingBudget := baseTemperature, thinkingBudgetTokens
	var last error
	for attempt := 0; attempt < geminiAttempts; attempt++ {
		// Backoff dipasang SEBELUM percobaan, bukan sesudah. Versi lama
		// menjalankan select/time.After tanpa syarat, sehingga percobaan
		// terakhir menunggu 4 detik hanya untuk mengembalikan error yang
		// sudah dipegangnya.
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			}
		}

		// Payload dibangun ulang di dalam loop. Sebelumnya di-marshal sekali
		// di luar, jadi ketiga percobaan mengirim byte identik pada suhu yang
		// sama: kegagalan validasi dijamin terulang persis sama sampai bulan
		// itu ditandai gagal.
		payload, _ := json.Marshal(buildRequestBody(input, temperature, thinkingBudget))
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", s.apiKey)

		res, err := s.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			// Badan respons ikut dibawa: pada 400, di situlah Gemini
			// menyebutkan field mana yang ditolak. Tanpa itu, kegagalan
			// konfigurasi skema hanya terlihat sebagai "HTTP 400" di kolom
			// error_message. Dipotong karena Fail() menyimpan 500 karakter.
			last = fmt.Errorf("Gemini HTTP %d: %s", res.StatusCode, firstLine(data, 300))
			if res.StatusCode != 429 && res.StatusCode < 500 {
				return nil, last
			}
			continue
		}

		analysis, err := parseGemini(data)
		if err == nil {
			return analysis, nil
		}
		last = err

		switch {
		case errors.Is(err, errGeminiRefused):
			// Penolakan tidak transien. Mengulanginya hanya membakar jatah
			// waktu grup ini dan menunda semua grup sesudahnya.
			return nil, last
		case errors.Is(err, errGeminiTruncated):
			thinkingBudget = 0
		default:
			temperature += temperatureStep
		}
	}
	return nil, last
}

func firstLine(data []byte, limit int) string {
	text := strings.Join(strings.Fields(string(data)), " ")
	if len(text) > limit {
		text = text[:limit] + "..."
	}
	return text
}

var (
	errGeminiTruncated = errors.New("Gemini response truncated")
	errGeminiRefused   = errors.New("Gemini refused to answer")
)

func parseGemini(data []byte) (*InsightAnalysis, error) {
	var response struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if len(response.Candidates) == 0 {
		return nil, errors.New("Gemini returned no analysis")
	}
	candidate := response.Candidates[0]
	// Tanpa membaca finishReason, pemotongan MAX_TOKENS muncul sebagai
	// "unexpected end of JSON input" yang buram lalu diulang secara identik
	// sampai habis percobaan.
	switch candidate.FinishReason {
	case "MAX_TOKENS":
		return nil, errGeminiTruncated
	case "SAFETY", "RECITATION", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII":
		return nil, fmt.Errorf("%w: %s", errGeminiRefused, candidate.FinishReason)
	}
	if len(candidate.Content.Parts) == 0 {
		return nil, errors.New("Gemini returned no analysis")
	}
	var a InsightAnalysis
	if err := json.Unmarshal([]byte(candidate.Content.Parts[0].Text), &a); err != nil {
		return nil, err
	}
	if err := validateAnalysis(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

func validateAnalysis(a *InsightAnalysis) error {
	valid := a.HealthStatus == "good" || a.HealthStatus == "watch" || a.HealthStatus == "risk"
	if !valid {
		return errors.New("invalid health status")
	}
	if strings.TrimSpace(a.Headline) == "" || len([]rune(a.Headline)) > maxHeadlineRunes || strings.TrimSpace(a.Summary) == "" || len([]rune(a.Summary)) > maxSummaryRunes {
		return errors.New("invalid analysis text")
	}
	if len(a.KeyFindings) > maxKeyFindings || len(a.Recommendations) > maxRecommendations || len(a.Cautions) > maxCautions {
		return errors.New("analysis contains too many items")
	}
	// Slice nil di-serialize menjadi `null`, bukan `[]`. Klien yang memakai
	// nilai default parameter (yang hanya menangkap `undefined`) akan crash
	// saat membaca .length. Normalkan di sini supaya bentuk JSON yang
	// tersimpan maupun yang dikirim selalu berupa array.
	if a.KeyFindings == nil {
		a.KeyFindings = []string{}
	}
	if a.Recommendations == nil {
		a.Recommendations = []InsightRecommendation{}
	}
	if a.Cautions == nil {
		a.Cautions = []string{}
	}
	return nil
}

func insightResponse(i *repository.AIInsight) (*InsightResponse, error) {
	var facts InsightFacts
	if err := json.Unmarshal(i.Facts, &facts); err != nil {
		return nil, err
	}
	var analysis *InsightAnalysis
	if len(i.Analysis) > 0 && string(i.Analysis) != "null" {
		var a InsightAnalysis
		if err := json.Unmarshal(i.Analysis, &a); err != nil {
			return nil, err
		}
		analysis = &a
	}
	return &InsightResponse{Status: i.Status, Period: i.PeriodMonth.Format("2006-01"), Facts: facts, Analysis: analysis, GeneratedAt: i.GeneratedAt, Model: i.Model, IsStale: i.GeneratedAt == nil || time.Since(*i.GeneratedAt) > 35*24*time.Hour, Error: i.ErrorMessage}, nil
}
func normalizeMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, jakartaLocation())
}

// previousMonth mengembalikan tanggal 1 bulan sebelum `now` (zona Jakarta).
//
// Urutannya WAJIB normalisasi dulu, baru kurangi bulan. Kebalikannya salah:
// AddDate menormalkan tanggal yang melimpah, sehingga pada tanggal 29-31
// pengurangan bulan justru menghasilkan bulan berjalan. Contoh nyata,
// 31 Desember 2026:
//
//	AddDate dulu  : Date(2026,11,31) -> dinormalkan jadi 2026-12-01  (SALAH)
//	normalize dulu: Date(2026,12,1)  -> AddDate(0,-1,0) -> 2026-11-01 (benar)
//
// Tanpa ini, insight bulan November tidak pernah dibuat, dan yang tersimpan
// justru analisis Desember yang datanya belum lengkap.
func previousMonth(now time.Time) time.Time {
	return normalizeMonth(now.In(jakartaLocation())).AddDate(0, -1, 0)
}

// NextInsightRun mengembalikan jadwal sapuan berikutnya: tanggal 1 pukul
// 00:01 waktu Jakarta.
//
// Sebelumnya scheduler memakai ticker 24 jam dari waktu boot, sehingga jam
// jalannya ikut jam deploy dan bergeser setiap kali server di-restart.
// Tanggal 1 dipilih karena GeneratePreviousMonthForEnabled menganalisis bulan
// SEBELUMNYA — pada tanggal 1 bulan itu baru saja tertutup dan datanya sudah
// lengkap. Menit ke-1, bukan ke-0, memberi jarak dari tengah malam tepat.
func NextInsightRun(now time.Time) time.Time {
	jakarta := jakartaLocation()
	local := now.In(jakarta)
	next := time.Date(local.Year(), local.Month(), 1, 0, 1, 0, 0, jakarta)
	if !next.After(local) {
		next = next.AddDate(0, 1, 0)
	}
	return next
}

func jakartaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}
