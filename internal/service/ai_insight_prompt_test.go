package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"personal-finance-tracker/internal/repository"
)

// TestPromptQuotesValidatorLimits menjaga agar batas yang diberitahukan ke
// model sama dengan batas yang ditegakkan validateAnalysis.
//
// Regresi yang dijaga: prompt lama tidak menyebut satu pun batas, sehingga
// respons yang kepanjangan ditolak validateAnalysis, diulang, ditolak lagi,
// lalu bulan itu ditandai gagal — tanpa sebab yang terlihat di log.
func TestPromptQuotesValidatorLimits(t *testing.T) {
	for _, want := range []string{
		fmt.Sprintf("MAKSIMAL %d karakter", maxHeadlineRunes),
		fmt.Sprintf("MAKSIMAL %d karakter", maxSummaryRunes),
		fmt.Sprintf("3 sampai %d butir", maxKeyFindings),
		fmt.Sprintf("2 sampai %d butir", maxRecommendations),
		fmt.Sprintf("0 sampai %d butir", maxCautions),
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("prompt tidak menyebut batas %q.\nKalau kata-katanya sengaja diubah, perbarui anchor di test ini; kalau konstantanya yang berubah, prompt ikut berubah otomatis.", want)
		}
	}
}

// TestPromptTemplateUsesPlaceholders menutup satu-satunya jalur drift yang
// masih tersisa. Batas tidak bisa menyimpang lewat konstanta — prompt dirender
// DARI konstanta itu — tapi masih bisa menyimpang kalau seseorang mengetik
// angkanya langsung di berkas .md. Test ini memeriksa sumber templatnya,
// bukan hasil rendernya.
func TestPromptTemplateUsesPlaceholders(t *testing.T) {
	for _, placeholder := range []string{
		"{{.MaxHeadlineRunes}}", "{{.MaxSummaryRunes}}",
		"{{.MaxKeyFindings}}", "{{.MaxRecommendations}}", "{{.MaxCautions}}",
	} {
		if !strings.Contains(promptTemplateRaw, placeholder) {
			t.Errorf("prompts/ai_insight_system.id.md tidak memakai %s — angkanya kemungkinan diketik langsung, dan akan menyimpang dari validateAnalysis", placeholder)
		}
	}
}

func TestPromptHasNoUnrenderedPlaceholder(t *testing.T) {
	if strings.Contains(systemPrompt, "{{") {
		t.Fatal("ada placeholder yang tidak tergantikan di prompts/ai_insight_system.id.md — tambahkan kuncinya di renderSystemPrompt")
	}
}

// TestSystemPromptExplainsEveryFactField gagal begitu ada field baru di
// InsightFacts yang tidak didokumentasikan di prompt. Model yang menerima
// field tanpa penjelasan akan menebak artinya.
func TestSystemPromptExplainsEveryFactField(t *testing.T) {
	value := reflect.TypeOf(InsightFacts{})
	for i := 0; i < value.NumField(); i++ {
		tag := strings.Split(value.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if !strings.Contains(systemPrompt, tag) {
			t.Errorf("prompt tidak menjelaskan facts.%s", tag)
		}
	}
}

func TestAnalysisSchemaMatchesValidatorLimits(t *testing.T) {
	properties := analysisSchema()["properties"].(map[string]any)
	cases := []struct {
		property, key string
		want          int
	}{
		{"key_findings", "maxItems", maxKeyFindings},
		{"recommendations", "maxItems", maxRecommendations},
		{"cautions", "maxItems", maxCautions},
		{"headline", "maxLength", maxHeadlineRunes},
		{"summary", "maxLength", maxSummaryRunes},
	}
	for _, tc := range cases {
		got := properties[tc.property].(map[string]any)[tc.key]
		if got != tc.want {
			t.Errorf("%s.%s = %v, harusnya %d", tc.property, tc.key, got, tc.want)
		}
	}
}

// TestSchemaCoversAnalysisStruct menangkap kelas bug yang sesungguhnya:
// menambah field di InsightAnalysis lalu lupa di skema (model tidak pernah
// mengisinya), atau menambah di skema lalu lupa di propertyOrdering (diam-diam
// kembali ke urutan alfabetis — persis bug yang ada sebelum perubahan ini).
func TestSchemaCoversAnalysisStruct(t *testing.T) {
	schema := analysisSchema()
	properties := schema["properties"].(map[string]any)

	var tags []string
	value := reflect.TypeOf(InsightAnalysis{})
	for i := 0; i < value.NumField(); i++ {
		tags = append(tags, strings.Split(value.Field(i).Tag.Get("json"), ",")[0])
	}

	for _, tag := range tags {
		if _, ok := properties[tag]; !ok {
			t.Errorf("field %q ada di InsightAnalysis tapi tidak di properties skema", tag)
		}
	}
	if len(properties) != len(tags) {
		t.Errorf("properties punya %d field, InsightAnalysis punya %d", len(properties), len(tags))
	}

	assertPermutation(t, "top level", schema["propertyOrdering"].([]string), keysOf(properties))
	assertPermutation(t, "top level required", schema["required"].([]string), keysOf(properties))

	item := properties["recommendations"].(map[string]any)["items"].(map[string]any)
	itemProperties := item["properties"].(map[string]any)
	assertPermutation(t, "recommendations", item["propertyOrdering"].([]string), keysOf(itemProperties))
}

// TestOrderingSurvivesJSONMarshal membuktikan perbaikannya lewat jalur
// serialisasi yang sungguhan, karena bug aslinya murni artefak marshalling:
// encoding/json mengurutkan key map, sehingga cautions terkirim paling awal
// dan model diminta menyimpulkan peringatan sebelum menganalisis apa pun.
func TestOrderingSurvivesJSONMarshal(t *testing.T) {
	data, err := json.Marshal(analysisSchema())
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		PropertyOrdering []string `json:"propertyOrdering"`
		Properties       struct {
			Recommendations struct {
				Items struct {
					PropertyOrdering []string `json:"propertyOrdering"`
				} `json:"items"`
			} `json:"recommendations"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	want := []string{"key_findings", "health_status", "headline", "summary", "recommendations", "cautions"}
	if !reflect.DeepEqual(decoded.PropertyOrdering, want) {
		t.Fatalf("propertyOrdering = %v, harusnya %v", decoded.PropertyOrdering, want)
	}
	wantItem := []string{"title", "action", "priority"}
	if got := decoded.Properties.Recommendations.Items.PropertyOrdering; !reflect.DeepEqual(got, wantItem) {
		t.Fatalf("propertyOrdering rekomendasi = %v, harusnya %v", got, wantItem)
	}
}

func TestValidateAnalysisBoundaries(t *testing.T) {
	valid := func() *InsightAnalysis {
		return &InsightAnalysis{Headline: "judul", Summary: "ringkasan", HealthStatus: "good"}
	}
	cases := []struct {
		name    string
		mutate  func(*InsightAnalysis)
		wantErr bool
	}{
		{"headline tepat di batas", func(a *InsightAnalysis) { a.Headline = strings.Repeat("a", maxHeadlineRunes) }, false},
		{"headline lewat satu", func(a *InsightAnalysis) { a.Headline = strings.Repeat("a", maxHeadlineRunes+1) }, true},
		{"summary tepat di batas", func(a *InsightAnalysis) { a.Summary = strings.Repeat("a", maxSummaryRunes) }, false},
		{"summary lewat satu", func(a *InsightAnalysis) { a.Summary = strings.Repeat("a", maxSummaryRunes+1) }, true},
		{"key_findings tepat di batas", func(a *InsightAnalysis) { a.KeyFindings = make([]string, maxKeyFindings) }, false},
		{"key_findings lewat satu", func(a *InsightAnalysis) { a.KeyFindings = make([]string, maxKeyFindings+1) }, true},
		{"recommendations tepat di batas", func(a *InsightAnalysis) {
			a.Recommendations = make([]InsightRecommendation, maxRecommendations)
		}, false},
		{"recommendations lewat satu", func(a *InsightAnalysis) {
			a.Recommendations = make([]InsightRecommendation, maxRecommendations+1)
		}, true},
		{"cautions tepat di batas", func(a *InsightAnalysis) { a.Cautions = make([]string, maxCautions) }, false},
		{"cautions lewat satu", func(a *InsightAnalysis) { a.Cautions = make([]string, maxCautions+1) }, true},
		{"headline hanya spasi", func(a *InsightAnalysis) { a.Headline = "   " }, true},
		{"summary kosong", func(a *InsightAnalysis) { a.Summary = "" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := valid()
			tc.mutate(a)
			if err := validateAnalysis(a); (err != nil) != tc.wantErr {
				t.Fatalf("validateAnalysis error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestValidateAnalysisCountsRunesNotBytes menjaga len([]rune(...)), bukan
// len(...). Headline Indonesia yang memuat karakter multi-byte akan ditolak
// keliru kalau seseorang "menyederhanakan" ini ke perhitungan byte.
func TestValidateAnalysisCountsRunesNotBytes(t *testing.T) {
	headline := strings.Repeat("é", maxHeadlineRunes)
	if len(headline) <= maxHeadlineRunes {
		t.Fatal("fixture tidak multi-byte, test ini tidak menguji apa pun")
	}
	a := &InsightAnalysis{Headline: headline, Summary: "ringkasan", HealthStatus: "good"}
	if err := validateAnalysis(a); err != nil {
		t.Fatalf("headline %d rune ditolak: %v", maxHeadlineRunes, err)
	}
}

// TestValidateAnalysisNormalisesNilSlices menjaga regresi yang pernah membuat
// halaman Reports crash: slice nil di-serialize jadi `null`, dan klien membaca
// .length di atasnya.
func TestValidateAnalysisNormalisesNilSlices(t *testing.T) {
	a := &InsightAnalysis{Headline: "judul", Summary: "ringkasan", HealthStatus: "good"}
	if err := validateAnalysis(a); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(a)
	for _, field := range []string{`"key_findings":[]`, `"recommendations":[]`, `"cautions":[]`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("%s tidak dinormalkan menjadi array kosong: %s", field, data)
		}
	}
}

func TestBuildRequestBodySeparatesInstructionFromData(t *testing.T) {
	body := buildRequestBody([]byte(`{"period":"2026-07"}`), baseTemperature, thinkingBudgetTokens)

	instruction := body["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(instruction, "RUBRIK health_status") {
		t.Error("systemInstruction tidak memuat rubrik")
	}

	content := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(content, "RUBRIK") {
		t.Error("instruksi bocor ke contents; seluruh aturan harus tinggal di systemInstruction")
	}
	if !strings.Contains(content, `{"period":"2026-07"}`) {
		t.Error("contents tidak memuat payload data")
	}

	config := body["generationConfig"].(map[string]any)
	if config["maxOutputTokens"] != maxOutputTokens {
		t.Errorf("maxOutputTokens = %v, harusnya %d", config["maxOutputTokens"], maxOutputTokens)
	}
	if config["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v", config["responseMimeType"])
	}
	// thinkingBudget WAJIB eksplisit. Dibiarkan dinamis, thinking bisa
	// memakan jatah maxOutputTokens sampai JSON terpotong di tengah.
	thinking := config["thinkingConfig"].(map[string]any)
	if thinking["thinkingBudget"] != thinkingBudgetTokens {
		t.Errorf("thinkingBudget = %v, harusnya %d", thinking["thinkingBudget"], thinkingBudgetTokens)
	}
}

// TestBuildInsightInputIncludesPeriod menjaga array nama bulan. Off-by-one di
// sini mengirim nama bulan yang salah ke SEMUA pengguna selama sebulan penuh,
// dan labelnya harus cocok dengan toLocaleDateString('id-ID') di frontend.
func TestBuildInsightInputIncludesPeriod(t *testing.T) {
	want := []string{
		"Januari 2026", "Februari 2026", "Maret 2026", "April 2026", "Mei 2026", "Juni 2026",
		"Juli 2026", "Agustus 2026", "September 2026", "Oktober 2026", "November 2026", "Desember 2026",
	}
	for i, label := range want {
		month := normalizeMonth(time.Date(2026, time.Month(i+1), 1, 0, 0, 0, 0, jakartaLocation()))
		in := buildInsightInput(month, InsightFacts{}, nil, nil)
		if in.PeriodLabel != label {
			t.Errorf("period_label bulan %d = %q, harusnya %q", i+1, in.PeriodLabel, label)
		}
		if got := fmt.Sprintf("2026-%02d", i+1); in.Period != got {
			t.Errorf("period = %q, harusnya %q", in.Period, got)
		}
		if in.Currency != "IDR" {
			t.Errorf("currency = %q", in.Currency)
		}
	}
}

func TestBuildInsightInputDaysInPeriod(t *testing.T) {
	cases := map[string]int{"2026-02": 28, "2024-02": 29, "2026-04": 30, "2026-01": 31}
	for period, want := range cases {
		parsed, _ := time.Parse("2006-01", period)
		in := buildInsightInput(normalizeMonth(parsed), InsightFacts{}, nil, nil)
		if in.DaysInPeriod != want {
			t.Errorf("%s: days_in_period = %d, harusnya %d", period, in.DaysInPeriod, want)
		}
	}
}

// TestBuildInsightInputHasPreviousMonth menjaga sinyal yang dipakai prompt
// untuk melarang model menyebut "bulan lalu" ketika memang tidak ada
// pembandingnya.
func TestBuildInsightInputHasPreviousMonth(t *testing.T) {
	month := normalizeMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, jakartaLocation()))
	if in := buildInsightInput(month, InsightFacts{}, nil, nil); in.HasPreviousMonth {
		t.Error("has_previous_month harus false saat tidak ada transaksi bulan sebelumnya")
	}
	previous := []repository.InsightTransaction{{Date: "2026-06-03", Type: "expense", Amount: 100, Category: "Makan"}}
	in := buildInsightInput(month, InsightFacts{}, nil, previous)
	if !in.HasPreviousMonth {
		t.Error("has_previous_month harus true")
	}
	if in.PreviousTotalExpense == nil || *in.PreviousTotalExpense != 100 {
		t.Errorf("previous_total_expense = %v", in.PreviousTotalExpense)
	}
}

// TestBuildCategoryChangesMarksNewCategory menjaga change_percent tetap null,
// bukan 0 dan bukan Inf. Memancarkan 0 untuk kategori yang baru muncul adalah
// cara mendapatkan "Listrik naik 0%" di produksi.
func TestBuildCategoryChangesMarksNewCategory(t *testing.T) {
	current := map[string]float64{"Listrik": 450000, "Makan": 3800000}
	previous := []repository.InsightTransaction{{Type: "expense", Amount: 2600000, Category: "Makan"}}

	changes := buildCategoryChanges(current, previous)
	byName := map[string]CategoryChange{}
	for _, c := range changes {
		byName[c.Name] = c
	}

	listrik := byName["Listrik"]
	if !listrik.IsNew || listrik.ChangePercent != nil {
		t.Errorf("kategori baru: is_new=%v change_percent=%v", listrik.IsNew, listrik.ChangePercent)
	}
	makan := byName["Makan"]
	if makan.IsNew || makan.ChangePercent == nil || *makan.ChangePercent != 46.2 {
		t.Errorf("Makan: is_new=%v change_percent=%v, harusnya 46.2", makan.IsNew, makan.ChangePercent)
	}
	// Diurutkan dari pergerakan terbesar, bukan nominal terbesar: daftar
	// terbesar-menurut-nominal sudah tersedia sebagai top_expense_categories.
	if changes[0].Name != "Makan" {
		t.Errorf("urutan pertama = %q, harusnya Makan (bergerak 1,2 juta)", changes[0].Name)
	}
}

// TestBuildCategoryChangesIncludesDisappearedCategory: kategori yang berhenti
// muncul adalah perubahan yang layak dilaporkan, bukan data yang hilang.
func TestBuildCategoryChangesIncludesDisappearedCategory(t *testing.T) {
	previous := []repository.InsightTransaction{{Type: "expense", Amount: 900000, Category: "Hiburan"}}
	changes := buildCategoryChanges(map[string]float64{}, previous)
	if len(changes) != 1 || changes[0].Name != "Hiburan" || changes[0].Amount != 0 {
		t.Fatalf("kategori yang hilang tidak dilaporkan: %+v", changes)
	}
	if changes[0].ChangePercent == nil || *changes[0].ChangePercent != -100 {
		t.Errorf("change_percent = %v, harusnya -100", changes[0].ChangePercent)
	}
}

// TestBoundedContextIsBounded adalah alasan utama dump transaksi mentah
// diganti. Sebelumnya seluruh transaksi bulan itu dikirim tanpa LIMIT, jadi
// biaya token satu grup tidak punya batas atas.
func TestBoundedContextIsBounded(t *testing.T) {
	month := normalizeMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, jakartaLocation()))
	categories := []string{"Makan", "Transportasi", "Listrik", "Hiburan", "Kesehatan", "Pendidikan", "Belanja", "Lainnya", "Pulsa", "Donasi", "Pajak", "Sewa", "Bensin", "Parkir", "Kopi"}

	var items []repository.InsightTransaction
	for i := 0; i < 5000; i++ {
		items = append(items, repository.InsightTransaction{
			Date:     fmt.Sprintf("2026-07-%02d", i%31+1),
			Type:     "expense",
			Amount:   float64(10000 + i),
			Category: categories[i%len(categories)],
		})
	}

	in := buildInsightInput(month, buildFacts(items, nil), items, nil)
	if len(in.CategoryChanges) > maxCategoryChanges {
		t.Errorf("category_changes = %d, batasnya %d", len(in.CategoryChanges), maxCategoryChanges)
	}
	if len(in.LargestExpenses) > maxLargestExpenses {
		t.Errorf("largest_expenses = %d, batasnya %d", len(in.LargestExpenses), maxLargestExpenses)
	}
	if len(in.Weekly) > maxWeeklyBuckets {
		t.Errorf("weekly = %d, batasnya %d", len(in.Weekly), maxWeeklyBuckets)
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 8192 {
		t.Errorf("payload %d byte untuk 5.000 transaksi; representasinya tidak lagi terkunci", len(data))
	}
	if strings.Contains(string(data), `"transactions"`) {
		t.Error("daftar transaksi mentah kembali masuk ke payload prompt")
	}
}

func TestBuildInsightInputDerivedStats(t *testing.T) {
	month := normalizeMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, jakartaLocation()))
	items := []repository.InsightTransaction{
		{Date: "2026-07-01", Type: "income", Amount: 1000, Category: "Gaji"},   // Rabu
		{Date: "2026-07-04", Type: "expense", Amount: 300, Category: "Makan"},  // Sabtu
		{Date: "2026-07-05", Type: "expense", Amount: 100, Category: "Makan"},  // Minggu
		{Date: "2026-07-09", Type: "expense", Amount: 600, Category: "Sewa"},   // Kamis
		{Date: "2026-07-09", Type: "expense", Amount: 200, Category: "Parkir"}, // hari sama
	}
	in := buildInsightInput(month, buildFacts(items, nil), items, nil)

	if in.IncomeTransactionCount != 1 || in.ExpenseTransactionCount != 4 {
		t.Errorf("cacah per tipe = %d income / %d expense", in.IncomeTransactionCount, in.ExpenseTransactionCount)
	}
	if in.ActiveDays != 4 {
		t.Errorf("active_days = %d, harusnya 4 tanggal berbeda", in.ActiveDays)
	}
	// 400 dari 1200 pengeluaran jatuh di akhir pekan.
	if in.WeekendExpenseSharePercent != 33.3 {
		t.Errorf("weekend_expense_share_percent = %v, harusnya 33.3", in.WeekendExpenseSharePercent)
	}
	if len(in.Weekly) != 2 || in.Weekly[0].Week != 1 || in.Weekly[1].Week != 2 {
		t.Errorf("bucket mingguan salah: %+v", in.Weekly)
	}
	if in.Weekly[0].Income != 1000 || in.Weekly[0].Expense != 400 {
		t.Errorf("pekan 1 = %+v", in.Weekly[0])
	}
	if in.LargestExpenses[0].Amount != 600 {
		t.Errorf("largest_expenses tidak diurutkan menurun: %+v", in.LargestExpenses)
	}
}

// TestPromptFingerprintFitsColumn: prompt_version adalah VARCHAR(32), dan
// kelebihan panjang baru muncul sebagai kegagalan insert di dalam Claim().
func TestPromptFingerprintFitsColumn(t *testing.T) {
	for _, env := range []string{"v2", "", "v99-experimental", strings.Repeat("x", 200)} {
		if got := effectivePromptVersion(env); len(got) > promptVersionMaxLength {
			t.Errorf("effectivePromptVersion(%d karakter) menghasilkan %d karakter", len(env), len(got))
		}
	}
}

// TestPromptFingerprintTracksPromptText adalah inti skema versi gabungan:
// mengubah teks prompt harus mengubah prompt_version dengan sendirinya,
// supaya Claim() meregenerasi tanpa perlu ada yang ingat menaikkan env var.
func TestPromptFingerprintTracksPromptText(t *testing.T) {
	before := effectivePromptVersion("v2")
	if !strings.HasPrefix(before, "v2+") {
		t.Fatalf("label manusia hilang dari versi: %q", before)
	}
	if before == effectivePromptVersion("v3") {
		t.Fatal("label env tidak berpengaruh pada versi efektif")
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func assertPermutation(t *testing.T, label string, got, want []string) {
	t.Helper()
	a, b := append([]string(nil), got...), append([]string(nil), want...)
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s: himpunan %v tidak sama dengan properties %v", label, got, want)
	}
}
