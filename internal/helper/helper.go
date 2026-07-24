// Package helper berisi fungsi-fungsi utilitas umum yang digunakan
// di berbagai layer aplikasi (handler, service, repository).
package helper

import (
	"net/mail"
	"strings"
	"time"
)

// ValidateEmail memeriksa apakah format email valid.
func ValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// FormatDate mengubah time.Time ke string dengan format YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// FormatDateTime mengubah time.Time ke string dengan format ISO8601.
func FormatDateTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// TruncateString memotong string hingga maxLength karakter.
// Jika string lebih panjang, ditambahkan "..." di akhir.
func TruncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength-3] + "..."
}

// NormalizeEmail mengubah email ke lowercase dan menghapus spasi.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ContainsString memeriksa apakah slice string mengandung suatu nilai.
func ContainsString(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// PointerToTime mengembalikan pointer ke time.Time (untuk nullable fields).
func PointerToTime(t time.Time) *time.Time {
	return &t
}

// SafeString mengembalikan string kosong jika pointer nil.
func SafeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
