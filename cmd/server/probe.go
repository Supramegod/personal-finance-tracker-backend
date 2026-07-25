package main

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── HTTP Handlers ────────────────────────────────

// HealthHandler adalah probe liveness untuk orchestrator.
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// ReadinessHandler adalah probe readiness — ping database.
func ReadinessHandler(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "not ready",
				"error":  "database unreachable",
			})
		}

		return c.JSON(fiber.Map{"status": "ready"})
	}
}

// ─── Background DB Health Check ────────────────────

// StartDBHealthCheck periodik ping database tiap interval.
func StartDBHealthCheck(pool *pgxpool.Pool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runDBHealthCheck(pool)

	for range ticker.C {
		runDBHealthCheck(pool)
	}
}

func runDBHealthCheck(pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		log.Printf("DB health check FAILED: %v", err)
	} else {
		log.Println("DB health check OK")
	}
}
