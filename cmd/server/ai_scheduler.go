package main

import (
	"context"
	"log"
	"time"

	"personal-finance-tracker/internal/service"
)

func StartAIInsightScheduler(insights *service.AIInsightService) {
	run := func() {
		// Tidak ada batas waktu untuk keseluruhan sapuan: setiap grup sudah
		// dibatasi sendiri di dalam GeneratePreviousMonthForEnabled. Batas
		// menyeluruh yang sebelumnya dipasang di sini justru berbahaya —
		// begitu habis, semua grup yang belum diproses ditandai gagal
		// hanya karena antreannya panjang, bukan karena ada yang salah.
		if err := insights.GeneratePreviousMonthForEnabled(context.Background(), time.Now()); err != nil {
			log.Printf("AI insight scheduler: %v", err)
		}
	}
	// Startup melakukan backfill idempotent untuk bulan sebelumnya.
	run()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
}
