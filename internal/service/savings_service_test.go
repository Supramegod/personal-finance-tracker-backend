package service

import (
	"errors"
	"math"
	"testing"
	"time"

	"personal-finance-tracker/internal/repository"
)

// Test untuk logika turunan tabungan. Semuanya murni perhitungan — tidak
// menyentuh database, jadi ikut jalan di `go test -short` tanpa infrastruktur.

func ptrFloat(v float64) *float64 { return &v }

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("tanggal fixture tidak valid %q: %v", value, err)
	}
	return parsed
}

func TestEnrichSavingsGoal(t *testing.T) {
	now := mustDate(t, "2026-08-09")
	created := mustDate(t, "2026-01-09")

	tests := []struct {
		name             string
		target           *float64
		targetDate       string
		saved            float64
		wantProgress     *float64
		wantRemaining    *float64
		wantMonthsLeft   *int
		wantSuggested    *float64
		wantOnTrackIsSet bool
		wantOnTrack      bool
	}{
		{
			// Celengan bebas: tanpa target tidak ada progres yang bisa dihitung.
			name:  "tanpa target tidak menghasilkan turunan apa pun",
			saved: 1_500_000,
		},
		{
			name:          "target tanpa tenggat hanya menghasilkan progres dan sisa",
			target:        ptrFloat(20_000_000),
			saved:         5_000_000,
			wantProgress:  ptrFloat(0.25),
			wantRemaining: ptrFloat(15_000_000),
		},
		{
			// Kasus dari dokumen konsep: sisa 8 bulan, sisa 5jt → 625rb/bulan.
			name:             "target dengan tenggat menghasilkan saran bulanan",
			target:           ptrFloat(20_000_000),
			targetDate:       "2027-04-09",
			saved:            15_000_000,
			wantProgress:     ptrFloat(0.75),
			wantRemaining:    ptrFloat(5_000_000),
			wantMonthsLeft:   ptrInt(8),
			wantSuggested:    ptrFloat(625_000),
			wantOnTrackIsSet: true,
			wantOnTrack:      true,
		},
		{
			name:             "target tercapai: progres mentok 1 dan sisa nol",
			target:           ptrFloat(10_000_000),
			targetDate:       "2027-04-09",
			saved:            12_000_000,
			wantProgress:     ptrFloat(1),
			wantRemaining:    ptrFloat(0),
			wantMonthsLeft:   ptrInt(8),
			wantSuggested:    ptrFloat(0),
			wantOnTrackIsSet: true,
			wantOnTrack:      true,
		},
		{
			// Tenggat lewat: months_left 0, jadi seluruh sisa disarankan sekarang
			// (bukan pembagian dengan nol).
			name:             "tenggat sudah lewat menyarankan seluruh sisa",
			target:           ptrFloat(10_000_000),
			targetDate:       "2026-05-09",
			saved:            4_000_000,
			wantProgress:     ptrFloat(0.4),
			wantRemaining:    ptrFloat(6_000_000),
			wantMonthsLeft:   ptrInt(0),
			wantSuggested:    ptrFloat(6_000_000),
			wantOnTrackIsSet: true,
			wantOnTrack:      false,
		},
		{
			name:             "tertinggal dari jadwal",
			target:           ptrFloat(12_000_000),
			targetDate:       "2027-01-09",
			saved:            1_000_000,
			wantProgress:     ptrFloat(1.0 / 12.0),
			wantRemaining:    ptrFloat(11_000_000),
			wantMonthsLeft:   ptrInt(5),
			wantSuggested:    ptrFloat(2_200_000),
			wantOnTrackIsSet: true,
			wantOnTrack:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &repository.SavingsGoalWithProgress{
				SavingsGoal: repository.SavingsGoal{
					TargetAmount: tt.target,
					CreatedAt:    created,
				},
				SavedAmount: tt.saved,
			}
			if tt.targetDate != "" {
				d := mustDate(t, tt.targetDate)
				item.TargetDate = &d
			}

			enrichSavingsGoal(item, now)

			assertFloatPtr(t, "progress", item.Progress, tt.wantProgress)
			assertFloatPtr(t, "remaining_amount", item.RemainingAmount, tt.wantRemaining)
			assertIntPtr(t, "months_left", item.MonthsLeft, tt.wantMonthsLeft)
			assertFloatPtr(t, "suggested_monthly", item.SuggestedMonthly, tt.wantSuggested)

			if tt.wantOnTrackIsSet {
				if item.IsOnTrack == nil {
					t.Fatalf("is_on_track: got nil; want %v", tt.wantOnTrack)
				}
				if *item.IsOnTrack != tt.wantOnTrack {
					t.Errorf("is_on_track: got %v; want %v", *item.IsOnTrack, tt.wantOnTrack)
				}
			} else if item.IsOnTrack != nil {
				t.Errorf("is_on_track: got %v; want nil", *item.IsOnTrack)
			}
		})
	}
}

func TestMonthsUntil(t *testing.T) {
	now := mustDate(t, "2026-08-09")

	tests := []struct {
		name   string
		target string
		want   int
	}{
		{"tenggat hari ini", "2026-08-09", 0},
		{"akhir bulan ini belum genap sebulan", "2026-08-31", 0},
		{"bulan depan tanggal sama", "2026-09-09", 1},
		{"bulan depan tanggal lebih awal belum genap", "2026-09-08", 0},
		{"delapan bulan lagi", "2027-04-09", 8},
		{"lintas tahun", "2027-08-09", 12},
		{"tenggat sudah lewat tidak pernah negatif", "2025-01-01", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := monthsUntil(now, mustDate(t, tt.target)); got != tt.want {
				t.Errorf("monthsUntil(%s): got %d; want %d", tt.target, got, tt.want)
			}
		})
	}
}

func TestParseOptionalDate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantNil bool
		wantErr bool
	}{
		{"kosong berarti tidak diisi, bukan error", "", true, false},
		{"spasi saja juga dianggap kosong", "   ", true, false},
		{"tanggal valid", "2027-04-09", false, false},
		{"format salah ditolak", "09-04-2027", false, true},
		{"bukan tanggal ditolak", "besok", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptionalDate(tt.value, "target_date")

			if tt.wantErr {
				if err == nil {
					t.Fatal("got nil error; want ValidationError")
				}
				// Handler membedakan error input dari kegagalan sistem lewat
				// errors.As — kalau tipenya salah, klien akan menerima 500.
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("got %T; want *ValidationError", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("got error %v; want nil", err)
			}
			if tt.wantNil && got != nil {
				t.Errorf("got %v; want nil", got)
			}
			if !tt.wantNil && got == nil {
				t.Error("got nil; want parsed date")
			}
		})
	}
}

func TestParseEntryDateDefaultsToToday(t *testing.T) {
	got, err := parseEntryDate("")
	if err != nil {
		t.Fatalf("got error %v; want nil", err)
	}
	wantY, wantM, wantD := time.Now().Date()
	gotY, gotM, gotD := got.Date()
	if gotY != wantY || gotM != wantM || gotD != wantD {
		t.Errorf("got %04d-%02d-%02d; want %04d-%02d-%02d", gotY, gotM, gotD, wantY, wantM, wantD)
	}
}

func TestIsValidSavingsStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"completed", true},
		{"archived", true},
		{"", false},
		{"Active", false},
		{"done", false},
	}

	for _, tt := range tests {
		t.Run("status="+tt.status, func(t *testing.T) {
			if got := isValidSavingsStatus(tt.status); got != tt.want {
				t.Errorf("got %v; want %v", got, tt.want)
			}
		})
	}
}

func ptrInt(v int) *int { return &v }

func assertFloatPtr(t *testing.T, field string, got, want *float64) {
	t.Helper()
	switch {
	case want == nil && got == nil:
		return
	case want == nil:
		t.Errorf("%s: got %v; want nil", field, *got)
	case got == nil:
		t.Errorf("%s: got nil; want %v", field, *want)
	case math.Abs(*got-*want) > 0.01:
		t.Errorf("%s: got %v; want %v", field, *got, *want)
	}
}

func assertIntPtr(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got == nil:
		return
	case want == nil:
		t.Errorf("%s: got %v; want nil", field, *got)
	case got == nil:
		t.Errorf("%s: got nil; want %v", field, *want)
	case *got != *want:
		t.Errorf("%s: got %d; want %d", field, *got, *want)
	}
}
