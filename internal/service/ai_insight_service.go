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

type AIInsightService struct {
	repo                         *repository.AIInsightRepository
	client                       *http.Client
	apiKey, model, promptVersion string
	enabled                      bool
}

func NewAIInsightService(repo *repository.AIInsightRepository, apiKey, model, promptVersion string, timeout time.Duration, enabled bool) *AIInsightService {
	return &AIInsightService{repo: repo, client: &http.Client{Timeout: timeout}, apiKey: apiKey, model: model, promptVersion: promptVersion, enabled: enabled}
}

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
		groupCtx, cancel := context.WithTimeout(ctx, perGroupTimeout)
		err := s.Generate(groupCtx, groupID, month)
		cancel()
		if err != nil { /* lanjutkan grup berikutnya */
			continue
		}
	}
	return nil
}

// perGroupTimeout membatasi satu grup dalam sapuan terjadwal.
//
// Batas atas kerja normal satu grup ~100 detik: 3 percobaan x timeout HTTP
// 30 detik, ditambah backoff 1s dan 2s. 3 menit memberi ruang aman tanpa
// membiarkan satu grup menggantung tak terbatas.
const perGroupTimeout = 3 * time.Minute

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

	analysis, err := s.callGemini(ctx, facts, items)
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

func (s *AIInsightService) callGemini(ctx context.Context, facts InsightFacts, items []repository.InsightTransaction) (*InsightAnalysis, error) {
	input, _ := json.Marshal(struct {
		Facts        InsightFacts                    `json:"facts"`
		Transactions []repository.InsightTransaction `json:"transactions"`
	}{facts, items})
	prompt := "Analisis data keuangan pribadi Indonesia berikut. Gunakan hanya fakta yang tersedia, jangan menciptakan angka, jangan memberi janji hasil, dan berikan saran praktis singkat dalam Bahasa Indonesia. Data: " + string(input)
	body := map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]any{"text": prompt}}}},
		"generationConfig": map[string]any{"temperature": 0.2, "responseMimeType": "application/json", "responseSchema": analysisSchema()},
	}
	payload, _ := json.Marshal(body)
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/" + url.PathEscape(s.model) + ":generateContent"
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", s.apiKey)
		res, err := s.client.Do(req)
		if err != nil {
			last = err
		} else {
			data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				analysis, err := parseGemini(data)
				if err == nil {
					return analysis, nil
				}
				last = err
			} else {
				last = fmt.Errorf("Gemini HTTP %d", res.StatusCode)
				if res.StatusCode != 429 && res.StatusCode < 500 {
					return nil, last
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}
	return nil, last
}

func parseGemini(data []byte) (*InsightAnalysis, error) {
	var response struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("Gemini returned no analysis")
	}
	var a InsightAnalysis
	if err := json.Unmarshal([]byte(response.Candidates[0].Content.Parts[0].Text), &a); err != nil {
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
	if strings.TrimSpace(a.Headline) == "" || len([]rune(a.Headline)) > 120 || strings.TrimSpace(a.Summary) == "" || len([]rune(a.Summary)) > 700 {
		return errors.New("invalid analysis text")
	}
	if len(a.KeyFindings) > 5 || len(a.Recommendations) > 5 || len(a.Cautions) > 3 {
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

func analysisSchema() map[string]any {
	return map[string]any{"type": "OBJECT", "properties": map[string]any{
		"headline": map[string]any{"type": "STRING"}, "summary": map[string]any{"type": "STRING"}, "health_status": map[string]any{"type": "STRING", "enum": []string{"good", "watch", "risk"}},
		"key_findings":    map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}, "maxItems": 5},
		"recommendations": map[string]any{"type": "ARRAY", "items": map[string]any{"type": "OBJECT", "properties": map[string]any{"title": map[string]any{"type": "STRING"}, "action": map[string]any{"type": "STRING"}, "priority": map[string]any{"type": "STRING", "enum": []string{"low", "medium", "high"}}}, "required": []string{"title", "action", "priority"}}, "maxItems": 5},
		"cautions":        map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}, "maxItems": 3}}, "required": []string{"headline", "summary", "health_status", "key_findings", "recommendations", "cautions"}}
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
func jakartaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return loc
}
