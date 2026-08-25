package service

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Batas keluaran model. Satu-satunya sumber kebenaran untuk tiga tempat yang
// WAJIB sepakat: teks prompt, responseSchema yang dikirim ke Gemini, dan
// validateAnalysis. Kalau ketiganya berbeda, model diberi tahu batas yang
// salah lalu jawabannya ditolak — pengguna melihat "Analisis gagal" tanpa
// sebab yang terlihat di log.
const (
	maxHeadlineRunes   = 120
	maxSummaryRunes    = 700
	maxKeyFindings     = 5
	maxRecommendations = 5
	maxCautions        = 3
)

// Anggaran token satu panggilan. Pada model Gemini 2.5 token thinking ikut
// dihitung terhadap maxOutputTokens, jadi keduanya harus ditetapkan bersama:
// thinking dinamis yang dibiarkan apa adanya bisa menghabiskan jatah,
// memotong JSON di tengah, dan menggagalkan parseGemini tiga kali berturut.
// Keluaran sah terpanjang sekitar 1.100 token.
const (
	maxOutputTokens        = 3072
	thinkingBudgetTokens   = 512
	promptVersionMaxLength = 32
)

//go:embed prompts/ai_insight_system.id.md
var promptTemplateRaw string

var systemPrompt = renderSystemPrompt()

func renderSystemPrompt() string {
	// missingkey=error WAJIB. Secara default, placeholder yang tidak
	// terdaftar di map dirender menjadi "<no value>" — bukan error dan
	// bukan "{{", sehingga lolos dari setiap pemeriksaan lalu terkirim
	// diam-diam ke Gemini di tengah instruksi.
	tpl := template.Must(template.New("ai_insight").Option("missingkey=error").Parse(promptTemplateRaw))
	var b strings.Builder
	if err := tpl.Execute(&b, map[string]int{
		"MaxHeadlineRunes":   maxHeadlineRunes,
		"MaxSummaryRunes":    maxSummaryRunes,
		"MaxKeyFindings":     maxKeyFindings,
		"MaxRecommendations": maxRecommendations,
		"MaxCautions":        maxCautions,
	}); err != nil {
		panic(err)
	}
	return b.String()
}

// analysisFieldOrder adalah urutan penalaran, bukan sekadar urutan tampilan.
//
// Untuk structured output, urutan pembangkitan field ADALAH urutan berpikir:
// key_findings lebih dulu memaksa angka konkret keluar sebelum ada prosa,
// health_status menyusul sebagai vonis di atas bukti itu, dan cautions
// terakhir karena definisinya adalah risiko sisa setelah rekomendasi —
// mustahil dihitung sebelum rekomendasinya ada.
var analysisFieldOrder = []string{"key_findings", "health_status", "headline", "summary", "recommendations", "cautions"}

var recommendationFieldOrder = []string{"title", "action", "priority"}

// analysisSchema mengunci bentuk keluaran Gemini.
//
// propertyOrdering WAJIB berupa []string, bukan map. encoding/json mengurutkan
// key map secara alfabetis, sehingga sebelum field ini ada urutan yang
// terkirim adalah cautions lebih dulu: model diminta menyimpulkan peringatan
// sebelum menganalisis apa pun, dan menetapkan health_status setelah headline
// terlanjur ditulis — badge dan judul bisa bercerita berlawanan.
func analysisSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"headline":      map[string]any{"type": "STRING", "maxLength": maxHeadlineRunes},
			"summary":       map[string]any{"type": "STRING", "maxLength": maxSummaryRunes},
			"health_status": map[string]any{"type": "STRING", "enum": []string{"good", "watch", "risk"}},
			// minItems 1, bukan 3: prompt meminta 3 butir pada kasus normal
			// tetapi tepat 1 pada bulan tanpa transaksi. Menaikkan batas bawah
			// di sini membuat kasus tepi itu bertabrakan dengan skema.
			"key_findings": map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}, "minItems": 1, "maxItems": maxKeyFindings},
			"recommendations": map[string]any{"type": "ARRAY", "minItems": 1, "maxItems": maxRecommendations, "items": map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"title":    map[string]any{"type": "STRING"},
					"action":   map[string]any{"type": "STRING"},
					"priority": map[string]any{"type": "STRING", "enum": []string{"low", "medium", "high"}},
				},
				"required":         recommendationFieldOrder,
				"propertyOrdering": recommendationFieldOrder,
			}},
			"cautions": map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}, "minItems": 0, "maxItems": maxCautions},
		},
		"required":         analysisFieldOrder,
		"propertyOrdering": analysisFieldOrder,
	}
}

// indonesianMonths harus persis sama dengan keluaran
// toLocaleDateString('id-ID', {month:'long'}) di AIInsightPanel.jsx, supaya
// prosa model sejalan dengan baris "Periode ..." yang dirender di sebelahnya.
// Go tidak punya lokalisasi nama bulan dan golang.org/x/text bukan dependensi.
var indonesianMonths = [12]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

func indonesianMonthLabel(t time.Time) string {
	return indonesianMonths[int(t.Month())-1] + " " + strconv.Itoa(t.Year())
}

// effectivePromptVersion menggabungkan label yang ditentukan manusia dengan
// sidik jari isi prompt.
//
// AI_PROMPT_VERSION hidup di tujuh berkas, enam di antaranya di luar perubahan
// Go apa pun. Tanpa sidik jari, perubahan prompt bisa ter-deploy di bawah
// string versi lama: Claim() tidak melihat perubahan, tidak pernah meng-klaim
// ulang, dan pengguna terus melihat keluaran lama selamanya sementara log
// menunjukkan deploy yang sukses. Kebalikannya sama buruknya — revert kode
// mengembalikan teks prompt tetapi tidak menyentuh string versi di .env.
//
// Hasilnya "v2+7f3a91c4". Dipotong agar muat di prompt_version VARCHAR(32);
// kelebihan panjang baru muncul sebagai kegagalan insert di dalam Claim(),
// jam dua pagi, di scheduler.
func effectivePromptVersion(envVersion string) string {
	sum := sha256.Sum256([]byte(systemPrompt))
	fingerprint := "+" + hex.EncodeToString(sum[:])[:8]
	if room := promptVersionMaxLength - len(fingerprint); len(envVersion) > room {
		envVersion = envVersion[:room]
	}
	return envVersion + fingerprint
}
