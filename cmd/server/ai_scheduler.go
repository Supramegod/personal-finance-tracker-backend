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
	//
	// Tetap dipertahankan meski jadwalnya sudah bulanan, justru karena
	// jadwalnya bulanan: deploy pada tanggal 15 — termasuk deploy yang
	// mengubah teks prompt — tidak perlu menunggu sampai tanggal 1 untuk
	// terlihat hasilnya. Claim() membuat pengulangan ini tidak berbiaya.
	run()

	for {
		next := service.NextInsightRun(time.Now())
		log.Printf("AI insight scheduler: sapuan berikutnya %s", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		<-timer.C
		run()
	}
}
