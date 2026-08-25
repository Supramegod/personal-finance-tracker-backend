package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"personal-finance-tracker/internal/repository"
)

// Eval prompt AI insight terhadap Gemini sungguhan.
//
//	GEMINI_API_KEY=... make test-ai-eval
//
// Digerbangi env var, BUKAN build tag: dengan build tag berkas ini hilang dari
// go vet di CI, padahal yang diinginkan sebaliknya — selalu ikut dikompilasi
// dan di-vet, hanya eksekusinya yang opsional. Pola yang sama dipakai
// RUN_INTEGRATION_TESTS di test/integration.

const evalConcurrency = 2

type evalFixture struct {
	name     string
	month    time.Time
	items    []repository.InsightTransaction
	previous []repository.InsightTransaction
	// forbid diperiksa terhadap seluruh teks keluaran, huruf kecil.
	forbid []string
	// checks untuk assertion khusus per skenario.
	checks func(t *testing.T, a *InsightAnalysis)
}

func evalMonth() time.Time {
	return normalizeMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, jakartaLocation()))
}

func tx(day int, kind, category string, amount float64) repository.InsightTransaction {
	return repository.InsightTransaction{
		Date:     fmt.Sprintf("2026-07-%02d", day),
		Type:     kind,
		Amount:   amount,
		Category: category,
	}
}

func prevTx(day int, kind, category string, amount float64) repository.InsightTransaction {
	return repository.InsightTransaction{
		Date:     fmt.Sprintf("2026-06-%02d", day),
		Type:     kind,
		Amount:   amount,
		Category: category,
	}
}

func spread(count int, kind, category string, amount float64) []repository.InsightTransaction {
	items := make([]repository.InsightTransaction, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, tx(i%28+1, kind, category, amount))
	}
	return items
}

func evalFixtures() []evalFixture {
	normalPrevious := []repository.InsightTransaction{
		prevTx(1, "income", "Gaji", 12000000),
		prevTx(5, "expense", "Makan", 2600000),
		prevTx(9, "expense", "Transportasi", 1900000),
		prevTx(14, "expense", "Listrik", 600000),
		prevTx(20, "expense", "Hiburan", 900000),
	}

	return []evalFixture{
		{
			name:  "normal_surplus",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(1, "income", "Gaji", 12000000),
				tx(4, "expense", "Makan", 3800000),
				tx(9, "expense", "Transportasi", 1900000),
				tx(14, "expense", "Listrik", 650000),
				tx(21, "expense", "Hiburan", 400000),
			},
			previous: normalPrevious,
		},
		{
			name:  "deficit",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(1, "income", "Gaji", 9000000),
				tx(3, "expense", "Makan", 4200000),
				tx(11, "expense", "Kesehatan", 5300000),
				tx(19, "expense", "Transportasi", 2000000),
			},
			previous: normalPrevious,
			// Larangan substring polos tidak dipakai di sini: "tidak sehat"
			// dan "belum aman" adalah kalimat yang BENAR untuk bulan tekor,
			// tetapi ikut tertangkap. Substansinya sudah dijaga subtest
			// rubrik (wajib "risk"); yang tersisa hanya memastikan judulnya
			// tidak mengklaim surplus.
			checks: func(t *testing.T, a *InsightAnalysis) {
				if strings.Contains(strings.ToLower(a.Headline), "surplus") {
					t.Errorf("bulan tekor tetapi judulnya mengklaim surplus: %q", a.Headline)
				}
			},
		},
		{
			name:     "zero_transactions",
			month:    evalMonth(),
			items:    nil,
			previous: normalPrevious,
			checks: func(t *testing.T, a *InsightAnalysis) {
				if len(a.KeyFindings) != 1 {
					t.Errorf("key_findings = %d butir, kasus tanpa transaksi harusnya tepat 1", len(a.KeyFindings))
				}
				if len(a.Cautions) != 0 {
					t.Errorf("cautions harus kosong, dapat %v", a.Cautions)
				}
				for _, category := range []string{"makan", "transportasi", "listrik", "hiburan"} {
					if strings.Contains(strings.ToLower(allText(a)), category) {
						t.Errorf("menyebut kategori %q padahal tidak ada transaksi", category)
					}
				}
			},
		},
		{
			name:  "zero_income",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(2, "expense", "Makan", 2800000),
				tx(12, "expense", "Transportasi", 1500000),
			},
			previous: normalPrevious,
			// savings_rate_percent bernilai 0 karena tidak bisa dihitung,
			// bukan karena kondisinya netral. Menyebutnya sebagai rasio
			// adalah kesalahan faktual.
			forbid: []string{"rasio tabungan"},
		},
		{
			name:  "first_month",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(1, "income", "Gaji", 8000000),
				tx(6, "expense", "Makan", 2400000),
				tx(15, "expense", "Sewa", 2000000),
			},
			previous: nil,
			// Tanpa bulan pembanding, setiap kata perbandingan adalah klaim
			// yang tidak punya dasar data.
			forbid: []string{"bulan lalu", "bulan sebelumnya", "dibanding", "naik", "turun"},
		},
		{
			name:  "dominant_category",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(1, "income", "Gaji", 10000000),
				tx(2, "expense", "Sewa", 5200000),
				tx(10, "expense", "Makan", 1200000),
				tx(18, "expense", "Transportasi", 700000),
			},
			previous: normalPrevious,
			checks: func(t *testing.T, a *InsightAnalysis) {
				if !strings.Contains(strings.ToLower(allText(a)), "sewa") {
					t.Error("kategori dominan (74% pengeluaran) tidak disebut sama sekali")
				}
			},
		},
		{
			name:  "expense_spike",
			month: evalMonth(),
			items: []repository.InsightTransaction{
				tx(1, "income", "Gaji", 12000000),
				tx(3, "expense", "Makan", 4100000),
				tx(8, "expense", "Belanja", 3900000),
				tx(16, "expense", "Transportasi", 2100000),
			},
			previous: normalPrevious,
		},
		{
			name:     "high_volume",
			month:    evalMonth(),
			items:    append(spread(400, "expense", "Makan", 25000), tx(1, "income", "Gaji", 15000000)),
			previous: normalPrevious,
		},
	}
}

func allText(a *InsightAnalysis) string {
	parts := []string{a.Headline, a.Summary}
	parts = append(parts, a.KeyFindings...)
	parts = append(parts, a.Cautions...)
	for _, r := range a.Recommendations {
		parts = append(parts, r.Title, r.Action)
	}
	return strings.Join(parts, "\n")
}

// expectedHealth adalah implementasi ulang rubrik di prompt. Assertion
// bersinyal paling tinggi di seluruh harness: ia mengukur langsung apakah
// model mematuhi instruksi eksplisit, bukan sekadar menghasilkan teks yang
// terdengar masuk akal.
func expectedHealth(f InsightFacts) string {
	switch {
	case f.TransactionCount == 0:
		return "watch"
	case f.TotalIncome == 0 && f.TotalExpense > 0:
		return "watch"
	case f.Net < 0:
		return "risk"
	}

	status := "risk"
	switch {
	case f.SavingsRate >= 20:
		status = "good"
	case f.SavingsRate >= 10:
		status = "watch"
	}

	if status != "good" {
		return status
	}
	if f.ExpenseChange != nil && *f.ExpenseChange > 25 {
		return "watch"
	}
	for _, c := range f.TopCategories {
		if c.Share > 50 {
			return "watch"
		}
	}
	return "good"
}

var (
	indonesianMarkers = []string{"yang", "dan", "untuk", "dari", "dengan", "pada", "tidak", "bulan", "lebih", "agar", "bisa", "masih"}
	englishLeak       = regexp.MustCompile(`(?i)\b(the|your|you|and|with|should|however|expenses|income|savings)\b`)
	bannedPhrases     = regexp.MustCompile(`(?i)(dijamin|pasti (untung|hemat|naik)|bebas risiko|tanpa risiko|saham|kripto|crypto|bitcoin|reksa ?dana|forex|trading|paylater|sebagai (ai|model)|boros|ceroboh)`)
	// Mekanisme penilaian bocor ke prosa pengguna. Ditemukan pada data nyata:
	// "Status turun menjadi perhatian karena ..." — pembaca tidak peduli pada
	// rubriknya, hanya pada temuannya.
	// Kata "status" saja yang dijadikan penanda, bukan daftar frasa: percobaan
	// sebelumnya memakai daftar dan model hanya berpindah bentuk kata —
	// "status turun" ditutup, keluar "status diturunkan" di field lain.
	rubricLeak   = regexp.MustCompile(`(?i)(\bstatus\b|sesuai rubrik|kategori "?(good|watch|risk)"?)`)
	markdownLeak = regexp.MustCompile(`(\*\*|^#|\x60)`)
)

type evalUsage struct {
	PromptTokens   int `json:"promptTokenCount"`
	OutputTokens   int `json:"candidatesTokenCount"`
	ThinkingTokens int `json:"thoughtsTokenCount"`
}

// evalOnce sengaja TIDAK memakai callGemini: yang diukur adalah keberhasilan
// pada PERCOBAAN PERTAMA. Retry akan menyembunyikan tepat cacat yang paling
// ingin dibuktikan hilang — respons yang melanggar batas lalu ditolak
// validateAnalysis sampai bulan itu ditandai gagal.
func evalOnce(ctx context.Context, model, apiKey string, body map[string]any) (*InsightAnalysis, evalUsage, error) {
	var usage evalUsage
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, usage, err
	}
	endpoint := fmt.Sprintf(geminiEndpointFm, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, usage, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	res, err := (&http.Client{Timeout: 150 * time.Second}).Do(req)
	if err != nil {
		return nil, usage, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, usage, fmt.Errorf("HTTP %d: %s", res.StatusCode, truncate(string(data), 400))
	}

	var meta struct {
		UsageMetadata evalUsage `json:"usageMetadata"`
	}
	_ = json.Unmarshal(data, &meta)
	usage = meta.UsageMetadata

	analysis, err := parseGemini(data)
	return analysis, usage, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func legacyPromptBody(input []byte) map[string]any {
	// Prompt v1 dibekukan DI BERKAS TEST, bukan di kode produksi, supaya
	// perbandingannya selamat dari perubahan produksi.
	const legacy = "Analisis data keuangan pribadi Indonesia berikut. Gunakan hanya fakta yang tersedia, jangan menciptakan angka, jangan memberi janji hasil, dan berikan saran praktis singkat dalam Bahasa Indonesia. Data: "
	return map[string]any{
		"contents": []any{map[string]any{"parts": []any{map[string]any{"text": legacy + string(input)}}}},
		"generationConfig": map[string]any{
			"temperature": 0.2, "responseMimeType": "application/json", "responseSchema": analysisSchema(),
		},
	}
}

type evalResult struct {
	Fixture     string
	Version     string
	SchemaOK    bool
	SchemaErr   string
	WantHealth  string
	GotHealth   string
	Ungrounded  []string
	Banned      []string
	Usage       evalUsage
	ElapsedMs   int64
	Headline    string
	SummaryHead string
}

func TestAIInsightEval(t *testing.T) {
	if testing.Short() || os.Getenv("RUN_AI_EVAL") != "1" {
		t.Skip("set RUN_AI_EVAL=1 (dan GEMINI_API_KEY) untuk menjalankan eval terhadap Gemini")
	}
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY belum diisi")
	}
	model := os.Getenv("AI_EVAL_MODEL")
	if model == "" {
		model = "gemini-flash-latest"
	}
	compareLegacy := os.Getenv("AI_EVAL_COMPARE") == "v1"

	gate := make(chan struct{}, evalConcurrency)
	results := make(chan evalResult, 64)

	for _, fixture := range evalFixtures() {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			gate <- struct{}{}
			defer func() { <-gate }()

			facts := buildFacts(fixture.items, fixture.previous)
			in := buildInsightInput(fixture.month, facts, fixture.items, fixture.previous)
			input, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}

			versions := map[string]map[string]any{"v2": buildRequestBody(input, baseTemperature, thinkingBudgetTokens)}
			if compareLegacy {
				versions["v1"] = legacyPromptBody(input)
			}

			for version, body := range versions {
				result := evalResult{Fixture: fixture.name, Version: version, WantHealth: expectedHealth(facts)}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				started := time.Now()
				analysis, usage, err := evalOnce(ctx, model, apiKey, body)
				cancel()
				result.ElapsedMs = time.Since(started).Milliseconds()
				result.Usage = usage

				if err != nil {
					result.SchemaErr = err.Error()
					results <- result
					if version == "v2" {
						t.Errorf("percobaan pertama gagal: %v", err)
					}
					continue
				}
				result.SchemaOK = true
				result.GotHealth = analysis.HealthStatus
				result.Headline = analysis.Headline
				result.SummaryHead = firstSentence(analysis.Summary)

				text := allText(analysis)
				result.Ungrounded = ungroundedNumbers(text, in)
				result.Banned = bannedPhrases.FindAllString(text, -1)
				results <- result

				if version != "v2" {
					continue
				}
				assertAnalysis(t, fixture, analysis, in, result)
			}
		})
	}

	t.Cleanup(func() {
		close(results)
		var all []evalResult
		for r := range results {
			all = append(all, r)
		}
		writeEvalReport(t, model, all)
	})
}

func assertAnalysis(t *testing.T, fixture evalFixture, a *InsightAnalysis, in insightInput, result evalResult) {
	t.Helper()
	text := allText(a)
	lower := strings.ToLower(text)

	t.Run("rubrik", func(t *testing.T) {
		if result.GotHealth != result.WantHealth {
			t.Errorf("health_status = %q, rubrik di prompt menuntut %q", result.GotHealth, result.WantHealth)
		}
	})

	t.Run("bahasa", func(t *testing.T) {
		found := 0
		for _, marker := range indonesianMarkers {
			if strings.Contains(lower, marker) {
				found++
			}
		}
		if found < 4 {
			t.Errorf("hanya %d penanda Bahasa Indonesia ditemukan", found)
		}
		if leaks := englishLeak.FindAllString(text, -1); len(leaks) > 0 {
			t.Errorf("kebocoran Bahasa Inggris: %v", leaks)
		}
	})

	t.Run("angka_terverifikasi", func(t *testing.T) {
		for _, problem := range result.Ungrounded {
			t.Errorf("angka tidak ada di masukan: %s", problem)
		}
	})

	t.Run("plafon_angka", func(t *testing.T) {
		ceiling := 1.05 * largestInputAmount(in)
		for _, token := range extractNumbers(text) {
			if token.Class == classMoney && token.Value > ceiling {
				t.Errorf("nominal %s melebihi plafon masukan %.0f", token.Raw, ceiling)
			}
		}
	})

	t.Run("frasa_terlarang", func(t *testing.T) {
		if len(result.Banned) > 0 {
			t.Errorf("frasa terlarang: %v", result.Banned)
		}
		if leaks := markdownLeak.FindAllString(text, -1); len(leaks) > 0 {
			t.Errorf("markdown bocor ke teks polos: %v", leaks)
		}
		if leaks := rubricLeak.FindAllString(text, -1); len(leaks) > 0 {
			t.Errorf("mekanisme penilaian bocor ke prosa pengguna: %v", leaks)
		}
		for _, forbidden := range fixture.forbid {
			pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(forbidden) + `\b`)
			if match := pattern.FindString(text); match != "" {
				t.Errorf("menyebut %q, padahal skenario ini melarangnya", match)
			}
		}
	})

	// Diturunkan langsung dari AIInsightPanel.jsx: mode ringkas merender
	// HANYA item.action untuk dua rekomendasi pertama, judulnya dibuang.
	t.Run("aksi_ringkas", func(t *testing.T) {
		for i, r := range a.Recommendations {
			if i >= 2 {
				break
			}
			runes := []rune(r.Action)
			if len(runes) < 20 || len(runes) > 180 {
				t.Errorf("recommendations[%d].action %d karakter: %q", i, len(runes), r.Action)
			}
			if len(runes) > 0 && unicode.IsLower(runes[0]) {
				t.Errorf("recommendations[%d].action diawali huruf kecil: %q", i, r.Action)
			}
			for _, opener := range []string{"ini ", "itu ", "hal ini", "dan ", "serta ", "sehingga "} {
				if strings.HasPrefix(strings.ToLower(r.Action), opener) {
					t.Errorf("recommendations[%d].action diawali %q sehingga tidak berdiri sendiri: %q", i, opener, r.Action)
				}
			}
		}
	})

	if fixture.checks != nil {
		t.Run("kasus_khusus", func(t *testing.T) { fixture.checks(t, a) })
	}
}

func firstSentence(s string) string {
	if index := strings.Index(s, ". "); index > 0 {
		return s[:index+1]
	}
	return s
}

func largestInputAmount(in insightInput) float64 {
	largest := in.Facts.TotalIncome
	for _, value := range []float64{in.Facts.TotalExpense, in.Facts.Net} {
		if value > largest {
			largest = value
		}
	}
	for _, c := range in.CategoryChanges {
		for _, value := range []float64{c.Amount, c.PreviousAmount} {
			if value > largest {
				largest = value
			}
		}
	}
	if in.PreviousTotalIncome != nil && *in.PreviousTotalIncome > largest {
		largest = *in.PreviousTotalIncome
	}
	return largest
}

func writeEvalReport(t *testing.T, model string, all []evalResult) {
	t.Helper()
	if len(all) == 0 {
		return
	}
	byFixture := map[string][]evalResult{}
	order := []string{}
	for _, r := range all {
		if _, seen := byFixture[r.Fixture]; !seen {
			order = append(order, r.Fixture)
		}
		byFixture[r.Fixture] = append(byFixture[r.Fixture], r)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Eval prompt AI insight\n\nModel: `%s`\n\n", model)
	for _, name := range order {
		fmt.Fprintf(&b, "## %s\n\n| | %s |\n|---|%s|\n", name,
			strings.Join(versionNames(byFixture[name]), " | "),
			strings.Repeat("---|", len(byFixture[name])))
		row := func(label string, value func(evalResult) string) {
			fmt.Fprintf(&b, "| %s |", label)
			for _, r := range byFixture[name] {
				fmt.Fprintf(&b, " %s |", value(r))
			}
			b.WriteString("\n")
		}
		row("validateAnalysis (percobaan 1)", func(r evalResult) string {
			if r.SchemaOK {
				return "OK"
			}
			return "GAGAL: " + truncate(r.SchemaErr, 90)
		})
		row("health_status", func(r evalResult) string { return r.GotHealth })
		row("rubrik", func(r evalResult) string {
			if !r.SchemaOK {
				return "-"
			}
			if r.GotHealth == r.WantHealth {
				return "cocok"
			}
			return "MELESET (harusnya " + r.WantHealth + ")"
		})
		row("angka tak terverifikasi", func(r evalResult) string { return fmt.Sprint(len(r.Ungrounded)) })
		row("frasa terlarang", func(r evalResult) string {
			if len(r.Banned) == 0 {
				return "-"
			}
			return strings.Join(r.Banned, ", ")
		})
		row("token prompt/keluaran/thinking", func(r evalResult) string {
			return fmt.Sprintf("%d / %d / %d", r.Usage.PromptTokens, r.Usage.OutputTokens, r.Usage.ThinkingTokens)
		})
		row("durasi", func(r evalResult) string { return fmt.Sprintf("%.1fs", float64(r.ElapsedMs)/1000) })
		row("headline", func(r evalResult) string { return "«" + r.Headline + "»" })
		row("kalimat pertama summary", func(r evalResult) string { return "«" + r.SummaryHead + "»" })
		b.WriteString("\n")
		for _, r := range byFixture[name] {
			for _, problem := range r.Ungrounded {
				fmt.Fprintf(&b, "- [%s] %s\n", r.Version, problem)
			}
		}
		b.WriteString("\n")
	}

	dir := filepath.Join("..", "..", "build", "ai-eval")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("tidak bisa membuat %s: %v", dir, err)
		t.Log(b.String())
		return
	}
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Logf("tidak bisa menulis laporan: %v", err)
	}
	t.Logf("laporan eval: %s\n\n%s", path, b.String())
}

func versionNames(results []evalResult) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Version)
	}
	return names
}
