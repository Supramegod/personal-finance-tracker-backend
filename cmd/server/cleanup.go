package main

import (
	"log"
	"time"

	"personal-finance-tracker/internal/repository"
)

// StartTokenCleanup menjalankan background goroutine untuk membersihkan
// refresh token yang sudah expired. Berjalan setiap 24 jam.
func StartTokenCleanup(repo *repository.RefreshTokenRepository) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	runTokenCleanup(repo)

	for range ticker.C {
		runTokenCleanup(repo)
	}
}

// runTokenCleanup menghapus semua refresh token yang sudah expired.
func runTokenCleanup(repo *repository.RefreshTokenRepository) {
	if err := repo.CleanupExpired(); err != nil {
		log.Printf("Token cleanup error: %v", err)
		return
	}

	log.Println("Token cleanup: expired refresh tokens purged")
}
