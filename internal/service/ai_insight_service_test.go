package service

import (
	"encoding/json"
	"errors"
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

// TestInsightPayloadContainsNoIdentityOrNote menelusuri SELURUH payload yang
// dikirim ke Gemini, bukan satu struct transaksi.
//
// Versi lama hanya memeriksa repository.InsightTransaction, sehingga tidak
// akan sadar kalau ada field identitas ditambahkan ke struct pembungkusnya —
// dan struct pembungkus itulah yang tumbuh setiap kali konteks diperkaya.
//
// Dialog consent menjanjikan hanya tanggal, kategori, tipe, dan nominal yang
// dikirim. "name" ikut dilarang dengan allowlist berbasis PATH, bukan
// dilewati begitu saja: nama kategori boleh, tetapi nama tujuan tabungan atau
// judul cicilan tidak — dan keduanya akan lolos kalau key "name" diabaikan
// secara buta.
func TestInsightPayloadContainsNoIdentityOrNote(t *testing.T) {
	month := normalizeMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, jakartaLocation()))
	items := []repository.InsightTransaction{
		{Date: "2026-07-01", Type: "income", Amount: 1000, Category: "Gaji"},
		{Date: "2026-07-04", Type: "expense", Amount: 300, Category: "Makan"},
	}
	previous := []repository.InsightTransaction{
		{Date: "2026-06-04", Type: "expense", Amount: 200, Category: "Hiburan"},
	}

	data, err := json.Marshal(buildInsightInput(month, buildFacts(items, previous), items, previous))
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}

	forbidden := map[string]bool{
		"id": true, "user_id": true, "group_id": true, "email": true,
		"note": true, "full_name": true, "title": true, "name": true,
		"goal_name": true, "description": true, "phone": true,
	}
	allowed := map[string]bool{
		"facts.top_expense_categories[].name": true,
		"category_changes[].name":             true,
	}

	walkJSONKeys(payload, "", func(path, key string) {
		if forbidden[key] && !allowed[path] {
			t.Errorf("payload membocorkan %q di %s", key, path)
		}
	})
}

func walkJSONKeys(node any, path string, visit func(path, key string)) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			visit(childPath, key)
			walkJSONKeys(child, childPath, visit)
		}
	case []any:
		for _, child := range value {
			walkJSONKeys(child, path+"[]", visit)
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

// TestNextInsightRunLandsOnFirstAt0001 menjaga jadwal sapuan.
//
// Sebelumnya scheduler memakai ticker 24 jam dari waktu boot, jadi jam
// jalannya mengikuti jam deploy dan bergeser setiap restart. Yang diuji di
// sini: selalu tanggal 1, selalu 00:01, selalu zona Jakarta, dan selalu di
// masa depan — termasuk saat dipanggil tepat pada detik jadwalnya.
func TestNextInsightRunLandsOnFirstAt0001(t *testing.T) {
	jakarta := jakartaLocation()
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"pertengahan bulan", time.Date(2026, 7, 15, 10, 0, 0, 0, jakarta), "2026-08-01T00:01"},
		{"tanggal 1 sebelum jadwal", time.Date(2026, 7, 1, 0, 0, 0, 0, jakarta), "2026-07-01T00:01"},
		{"tanggal 1 tepat di jadwal", time.Date(2026, 7, 1, 0, 1, 0, 0, jakarta), "2026-08-01T00:01"},
		{"tanggal 1 sesudah jadwal", time.Date(2026, 7, 1, 0, 2, 0, 0, jakarta), "2026-08-01T00:01"},
		{"akhir bulan pendek", time.Date(2026, 2, 28, 23, 59, 0, 0, jakarta), "2026-03-01T00:01"},
		{"pergantian tahun", time.Date(2026, 12, 31, 23, 59, 0, 0, jakarta), "2027-01-01T00:01"},
		// 31 Juli 18:00 UTC sudah 1 Agustus 01:00 WIB, jadi jadwal Agustus
		// terlewat dan yang berikutnya September.
		{"masukan UTC melewati batas bulan", time.Date(2026, 7, 31, 18, 0, 0, 0, time.UTC), "2026-09-01T00:01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextInsightRun(tc.now)
			if got.In(jakarta).Format("2006-01-02T15:04") != tc.want {
				t.Fatalf("NextInsightRun(%s) = %s, harusnya %s", tc.now.Format(time.RFC3339), got.In(jakarta).Format(time.RFC3339), tc.want)
			}
			if !got.After(tc.now) {
				t.Fatalf("jadwal %s tidak berada setelah %s", got, tc.now)
			}
		})
	}
}

// TestNextInsightRunPairsWithPreviousMonth: pada saat sapuan jalan, bulan yang
// dianalisis harus bulan yang baru saja tertutup.
func TestNextInsightRunPairsWithPreviousMonth(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, jakartaLocation())
	fires := NextInsightRun(now)
	if got := previousMonth(fires).Format("2006-01"); got != "2026-07" {
		t.Fatalf("sapuan %s menganalisis %s, harusnya 2026-07", fires.Format("2006-01-02"), got)
	}
}

// TestPerGroupTimeoutCoversEveryAttempt menjaga agar batas waktu per grup
// selalu memuat seluruh percobaan retry.
//
// Regresi yang dijaga: perGroupTimeout dulu konstan 3 menit sementara
// AI_TIMEOUT bisa diubah lewat env. Pada AI_TIMEOUT 60 detik, tiga percobaan
// butuh 183 detik dan scheduler memotongnya di tengah jalan lalu menandai
// grup itu gagal tanpa sebab yang terlihat.
func TestPerGroupTimeoutCoversEveryAttempt(t *testing.T) {
	for _, httpTimeout := range []time.Duration{30 * time.Second, 60 * time.Second, 2 * time.Minute} {
		svc := NewAIInsightService(nil, "kunci", "model", "v2", httpTimeout, true)
		worstCase := time.Duration(geminiAttempts)*httpTimeout + 3*time.Second
		if got := svc.perGroupTimeout(); got <= worstCase {
			t.Errorf("AI_TIMEOUT=%s: perGroupTimeout %s tidak memuat %s untuk %d percobaan", httpTimeout, got, worstCase, geminiAttempts)
		}
	}
}

// TestRegenerateRejectsUnfinishedMonth menjaga agar tombol "buat ulang" tidak
// bisa dipakai untuk bulan yang datanya belum lengkap.
//
// repo sengaja nil: penjaga bulan HARUS mengembalikan error sebelum menyentuh
// database sama sekali, jadi test ini juga membuktikan urutan pemeriksaannya.
func TestRegenerateRejectsUnfinishedMonth(t *testing.T) {
	svc := NewAIInsightService(nil, "kunci", "gemini-flash-lite-latest", "v2", 30*time.Second, true)
	now := time.Now().In(jakartaLocation())
	cases := map[string]time.Time{
		"bulan berjalan": normalizeMonth(now),
		"bulan depan":    normalizeMonth(now).AddDate(0, 1, 0),
		"tahun depan":    normalizeMonth(now).AddDate(1, 0, 0),
	}
	for name, month := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.Regenerate("grup", "pengguna", month); !errors.Is(err, ErrRegenerateFutureMonth) {
				t.Fatalf("Regenerate(%s) = %v, harusnya ErrRegenerateFutureMonth", month.Format("2006-01"), err)
			}
		})
	}
}

// TestRegenerateRequiresConfiguration: tanpa kunci API, permintaan ditolak
// sebelum ada baris yang diubah statusnya menjadi pending.
func TestRegenerateRequiresConfiguration(t *testing.T) {
	lastMonth := previousMonth(time.Now())
	for name, svc := range map[string]*AIInsightService{
		"fitur dimatikan": NewAIInsightService(nil, "kunci", "m", "v2", time.Second, false),
		"kunci kosong":    NewAIInsightService(nil, "", "m", "v2", time.Second, true),
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.Regenerate("grup", "pengguna", lastMonth); err == nil {
				t.Fatal("Regenerate harusnya ditolak saat AI insight tidak dikonfigurasi")
			}
		})
	}
}
