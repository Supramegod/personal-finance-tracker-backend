package main

import (
	"context"
	"log"
	"time"

	"personal-finance-tracker/internal/service"
)

func StartAIInsightScheduler(insights *service.AIInsightService) {
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		if err := insights.GeneratePreviousMonthForEnabled(ctx, time.Now()); err != nil {
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
