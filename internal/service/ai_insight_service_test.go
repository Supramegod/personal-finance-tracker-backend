package service

import (
	"encoding/json"
	"testing"
	"time"

	"personal-finance-tracker/internal/repository"
)

func TestBuildInsightFacts(t *testing.T) {
	previous := []repository.InsightTransaction{{Type: "expense", Amount: 100}}
	items := []repository.InsightTransaction{
		{Type: "income", Amount: 1000, Category: "Gaji"},
		{Type: "expense", Amount: 300, Category: "Makan"},
		{Type: "expense", Amount: 100, Category: "Transportasi"},
	}
	facts := buildFacts(items, previous)
	if facts.TotalIncome != 1000 || facts.TotalExpense != 400 || facts.Net != 600 {
		t.Fatalf("unexpected totals: %+v", facts)
	}
	if facts.SavingsRate != 60 || facts.ExpenseChange == nil || *facts.ExpenseChange != 300 {
		t.Fatalf("unexpected derived facts: %+v", facts)
	}
	if len(facts.TopCategories) != 2 || facts.TopCategories[0].Name != "Makan" {
		t.Fatalf("categories are not sorted: %+v", facts.TopCategories)
	}
}

func TestInsightPayloadContainsNoIdentityOrNote(t *testing.T) {
	data, err := json.Marshal(repository.InsightTransaction{Date: "2026-07-01", Type: "expense", Amount: 10, Category: "Makan"})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"id", "user_id", "email", "note", "full_name"} {
		if _, exists := value[forbidden]; exists {
			t.Fatalf("payload leaks %s", forbidden)
		}
	}
}

func TestParseGeminiStructuredOutput(t *testing.T) {
	analysis := `{"headline":"Arus kas sehat","summary":"Pengeluaran masih terkendali.","health_status":"good","key_findings":["Saldo positif"],"recommendations":[{"title":"Pertahankan","action":"Tinjau mingguan","priority":"low"}],"cautions":[]}`
	response, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": analysis}}}}}})
	result, err := parseGemini(response)
	if err != nil || result.HealthStatus != "good" {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestValidateAnalysisRejectsUnknownStatus(t *testing.T) {
	err := validateAnalysis(&InsightAnalysis{Headline: "x", Summary: "y", HealthStatus: "unknown"})
	if err == nil {
		t.Fatal("expected invalid health status")
	}
}

// TestPreviousMonthOnMonthEnd menjaga urutan normalisasi di previousMonth.
//
// Regresi yang dijaga: sebelumnya kode memanggil AddDate(0,-1,0) LEBIH DULU
// baru dinormalkan. Karena AddDate menormalkan tanggal yang melimpah
// (31 November tidak ada -> menjadi 1 Desember), setiap tanggal 29-31 pada
// bulan yang lebih pendek satu bulan sebelumnya menghasilkan bulan berjalan,
// bukan bulan sebelumnya. Akibatnya insight bulan lalu tidak pernah dibuat.
func TestPreviousMonthOnMonthEnd(t *testing.T) {
	jakarta := jakartaLocation()
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"31 Desember -> November", time.Date(2026, 12, 31, 23, 0, 0, 0, jakarta), "2026-11"},
		{"31 Maret -> Februari", time.Date(2026, 3, 31, 12, 0, 0, 0, jakarta), "2026-02"},
		{"31 Mei -> April", time.Date(2026, 5, 31, 12, 0, 0, 0, jakarta), "2026-04"},
		{"31 Juli -> Juni", time.Date(2026, 7, 31, 12, 0, 0, 0, jakarta), "2026-06"},
		{"31 Oktober -> September", time.Date(2026, 10, 31, 12, 0, 0, 0, jakarta), "2026-09"},
		{"29 Maret kabisat -> Februari", time.Date(2024, 3, 29, 12, 0, 0, 0, jakarta), "2024-02"},
		{"1 Januari -> Desember tahun lalu", time.Date(2026, 1, 1, 0, 0, 0, 0, jakarta), "2025-12"},
		{"15 Juni -> Mei (kasus biasa)", time.Date(2026, 6, 15, 12, 0, 0, 0, jakarta), "2026-05"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := previousMonth(tc.now)
			if got.Format("2006-01") != tc.want {
				t.Fatalf("previousMonth(%s) = %s, expected %s", tc.now.Format("2006-01-02"), got.Format("2006-01"), tc.want)
			}
			if got.Day() != 1 {
				t.Fatalf("expected the 1st of the month, got day %d", got.Day())
			}
		})
	}
}

// UTC menggeser tanggal mundur di awal bulan waktu Jakarta (WIB = UTC+7),
// jadi konversi zona waktu harus terjadi sebelum normalisasi.
func TestPreviousMonthUsesJakartaTimezone(t *testing.T) {
	// 1 Maret 2026 pukul 03:00 WIB masih 28 Februari 20:00 UTC.
	now := time.Date(2026, 3, 1, 3, 0, 0, 0, jakartaLocation()).UTC()
	if got := previousMonth(now); got.Format("2006-01") != "2026-02" {
		t.Fatalf("previousMonth = %s, expected 2026-02", got.Format("2006-01"))
	}
}
