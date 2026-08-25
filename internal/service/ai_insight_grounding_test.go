package service

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Pemeriksa grounding angka: memastikan setiap angka yang ditulis model
// benar-benar ada di data masukan.
//
// Prompt dan pemeriksa ini dirancang bersama. Aturan format rupiah di prompt
// ("Rp1.250.000", boleh diringkas "Rp8,4 juta", persen satu desimal dengan
// koma) ada justru supaya pembulatannya bisa diprediksi dan dicocokkan
// kembali di sini. Tanpa toleransi, "Rp8,4 juta" untuk 8.412.500 terbaca
// sebagai halusinasi dan orang akan mematikan pemeriksanya dalam seminggu.

type numberClass string

const (
	classMoney   numberClass = "money"
	classPercent numberClass = "percent"
	classCount   numberClass = "count"
)

type numberToken struct {
	Raw   string
	Value float64
	Class numberClass
}

var numberPattern = regexp.MustCompile(`(\d{1,3}(?:\.\d{3})+|\d+)(?:,(\d+))?\s*(%|juta|jt|ribu|rb|miliar)?`)

// rubricThresholds adalah angka ambang yang dinyatakan di rubrik prompt.
var rubricThresholds = []float64{10, 20, 25, 50}

var suffixMultiplier = map[string]float64{
	"juta": 1e6, "jt": 1e6, "ribu": 1e3, "rb": 1e3, "miliar": 1e9,
}

func extractNumbers(text string) []numberToken {
	var tokens []numberToken
	for _, match := range numberPattern.FindAllStringSubmatchIndex(text, -1) {
		group := func(i int) string {
			if match[2*i] < 0 {
				return ""
			}
			return text[match[2*i]:match[2*i+1]]
		}
		integer, fraction, suffix := group(1), group(2), strings.ToLower(group(3))

		digits := strings.ReplaceAll(integer, ".", "")
		if fraction != "" {
			digits += "." + fraction
		}
		value, err := strconv.ParseFloat(digits, 64)
		if err != nil {
			continue
		}

		token := numberToken{Raw: strings.TrimSpace(text[match[0]:match[1]]), Value: value}
		switch {
		case suffix == "%":
			token.Class = classPercent
		case suffixMultiplier[suffix] > 0:
			token.Class = classMoney
			token.Value *= suffixMultiplier[suffix]
		case hasRupiahPrefix(text, match[0]), strings.Contains(integer, "."):
			// "Rp1.250.000" dan bentuk berpemisah ribuan selalu nominal.
			token.Class = classMoney
		default:
			token.Class = classCount
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func hasRupiahPrefix(text string, start int) bool {
	prefix := strings.TrimSpace(text[:start])
	return strings.HasSuffix(prefix, "Rp") || strings.HasSuffix(prefix, "rp")
}

// allowedNumbers dibangun PERSIS dari nilai masukan, tanpa satu pun jumlah
// turunan. Prompt melarang model berhitung, jadi menerima hasil penjumlahan
// di sini akan melubangi aturan yang justru sedang diuji.
func allowedNumbers(in insightInput) map[numberClass][]float64 {
	allowed := map[numberClass][]float64{}
	money := func(values ...float64) {
		allowed[classMoney] = append(allowed[classMoney], values...)
	}
	percent := func(values ...float64) {
		for _, v := range values {
			allowed[classPercent] = append(allowed[classPercent], v, math.Abs(v))
		}
	}
	count := func(values ...float64) {
		allowed[classCount] = append(allowed[classCount], values...)
	}

	f := in.Facts
	money(f.TotalIncome, f.TotalExpense, f.Net, math.Abs(f.Net))
	percent(f.SavingsRate)
	if f.ExpenseChange != nil {
		percent(*f.ExpenseChange)
	}
	count(float64(f.TransactionCount))
	for _, c := range f.TopCategories {
		money(c.Amount)
		percent(c.Share)
	}
	for _, c := range in.CategoryChanges {
		money(c.Amount, c.PreviousAmount)
		if c.ChangePercent != nil {
			percent(*c.ChangePercent)
		}
	}
	for _, w := range in.Weekly {
		money(w.Expense, w.Income)
		count(float64(w.Week))
	}
	for _, e := range in.LargestExpenses {
		money(e.Amount)
	}
	if in.PreviousTotalIncome != nil {
		money(*in.PreviousTotalIncome)
	}
	if in.PreviousTotalExpense != nil {
		money(*in.PreviousTotalExpense)
	}
	percent(in.WeekendExpenseSharePercent, 100)
	// Ambang rubrik yang tertulis di prompt. Model yang menyebut "porsi di
	// atas 50%" sedang mengutip aturan yang menjelaskan vonisnya, bukan
	// mengarang angka — dan itu justru perilaku yang diinginkan.
	percent(rubricThresholds...)
	count(float64(in.ExpenseTransactionCount), float64(in.IncomeTransactionCount),
		float64(in.ActiveDays), float64(in.DaysInPeriod), 100)
	// Tanggal, nomor pekan, dan cacah kecil lain yang wajar muncul di prosa.
	for i := 0; i <= 31; i++ {
		count(float64(i))
	}
	if year, err := strconv.Atoi(strings.Split(in.Period, "-")[0]); err == nil {
		count(float64(year))
	}
	if month, err := strconv.Atoi(strings.Split(in.Period, "-")[1]); err == nil {
		count(float64(month))
	}
	return allowed
}

func matchesAllowed(token numberToken, allowed map[numberClass][]float64) bool {
	for _, candidate := range allowed[token.Class] {
		if withinTolerance(token, candidate) {
			return true
		}
	}
	return false
}

func withinTolerance(token numberToken, candidate float64) bool {
	diff := math.Abs(token.Value - candidate)
	switch token.Class {
	case classPercent:
		return diff <= 0.6
	case classMoney:
		// 1,5% menutup peringkasan "Rp8,4 juta" untuk 8.412.500; lantai
		// Rp1.000 menutup pembulatan ke rupiah penuh pada nominal kecil.
		return diff <= math.Max(0.015*math.Abs(candidate), 1000)
	default:
		return diff < 0.0001
	}
}

// ungroundedNumbers mengembalikan angka yang tidak bisa ditelusuri ke masukan,
// beserta kalimat asalnya supaya kegagalannya bisa didiagnosis tanpa
// menjalankan ulang eval.
func ungroundedNumbers(text string, in insightInput) []string {
	allowed := allowedNumbers(in)
	var problems []string
	for _, sentence := range splitSentences(text) {
		for _, token := range extractNumbers(sentence) {
			if !matchesAllowed(token, allowed) {
				problems = append(problems, fmt.Sprintf("%q (%s) dalam kalimat: %s", token.Raw, token.Class, strings.TrimSpace(sentence)))
			}
		}
	}
	return problems
}

func splitSentences(text string) []string {
	return regexp.MustCompile(`(?:\.\s+|\n)`).Split(text, -1)
}

// TestGroundingCheckerItself menguji pemeriksanya sendiri.
//
// Ini bukan formalitas: ekstraktor dengan regex rusak akan meloloskan SEMUA
// fixture secara diam-diam, dan seluruh harness eval berubah menjadi teater
// yang selalu hijau.
func TestGroundingCheckerItself(t *testing.T) {
	cases := []struct {
		text  string
		value float64
		class numberClass
	}{
		{"Rp1.250.000", 1250000, classMoney},
		{"Rp8,4 juta", 8400000, classMoney},
		{"Rp450 ribu", 450000, classMoney},
		{"sekitar Rp2.200.000 tersisa", 2200000, classMoney},
		{"27,8%", 27.8, classPercent},
		{"naik 46,2% dari", 46.2, classPercent},
		{"63 transaksi", 63, classCount},
		{"21 hari aktif", 21, classCount},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			tokens := extractNumbers(tc.text)
			if len(tokens) != 1 {
				t.Fatalf("dapat %d token dari %q: %+v", len(tokens), tc.text, tokens)
			}
			if tokens[0].Value != tc.value || tokens[0].Class != tc.class {
				t.Fatalf("%q -> %v (%s), harusnya %v (%s)", tc.text, tokens[0].Value, tokens[0].Class, tc.value, tc.class)
			}
		})
	}
}

func TestGroundingCheckerFlagsInventedNumbers(t *testing.T) {
	in := insightInput{
		Period: "2026-07",
		Facts: InsightFacts{
			TotalIncome: 12000000, TotalExpense: 9600000, Net: 2400000, SavingsRate: 20,
			TransactionCount: 63,
			TopCategories:    []CategorySpending{{Name: "Makan", Amount: 3800000, Share: 39.6}},
		},
	}

	grounded := "Pemasukan Rp12.000.000 dengan sisa Rp2,4 juta. Kategori Makan Rp3.800.000 atau 39,6%."
	if problems := ungroundedNumbers(grounded, in); len(problems) != 0 {
		t.Errorf("angka yang sah ditandai halusinasi: %v", problems)
	}

	invented := "Anda bisa menghemat Rp5.750.000 bulan depan."
	problems := ungroundedNumbers(invented, in)
	if len(problems) != 1 {
		t.Fatalf("angka karangan tidak tertangkap: %v", problems)
	}
	if !strings.Contains(problems[0], "5.750.000") {
		t.Errorf("laporan tidak menyebut angkanya: %s", problems[0])
	}
}

// TestGroundingCheckerToleratesRounding: prompt mengizinkan peringkasan ke
// satuan juta, jadi pemeriksanya harus menerimanya. Tanpa ini, setiap
// "Rp8,4 juta" jadi positif palsu.
func TestGroundingCheckerToleratesRounding(t *testing.T) {
	in := insightInput{Period: "2026-07", Facts: InsightFacts{TotalExpense: 8412500, SavingsRate: 27.83}}
	for _, text := range []string{"Pengeluaran Rp8,4 juta.", "Rasio tabungan 27,8%.", "Rasio tabungan 28%."} {
		if problems := ungroundedNumbers(text, in); len(problems) != 0 {
			t.Errorf("%q ditandai halusinasi: %v", text, problems)
		}
	}
}
