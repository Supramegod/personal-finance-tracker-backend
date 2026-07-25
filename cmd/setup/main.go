package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"personal-finance-tracker/internal/config"
	"personal-finance-tracker/internal/repository"
)

func main() {
	debug := flag.Bool("d", false, "sets log level to debug")
	seed := flag.Bool("seed", false, "seed admin user + default categories")
	migrate := flag.Bool("migrate", false, "run database migrations")
	cleanupTokens := flag.Bool("cleanup-tokens", false, "manually purge expired refresh tokens")
	listUsers := flag.Bool("list-users", false, "list all registered users")
	flag.Parse()

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system env vars")
	}

	// Load config
	cfg := config.Load()

	if *debug {
		log.Println("Debug mode enabled")
	}

	// DB connection — mirip pola user: pgxpool.New dengan timeout context
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	connPool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Can't connect to database: %v", err)
	}
	cancel()
	defer connPool.Close()

	// Repositories
	userRepo := repository.NewUserRepository(connPool)
	refreshTokenRepo := repository.NewRefreshTokenRepository(connPool)

	// ─── Run sesuai flag ───────────────────────────

	if *migrate {
		log.Println("Running migrations...")
		db, err := repository.NewDB(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("Failed to create DB connection for migration: %v", err)
		}
		if err := db.RunMigrations(); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		db.Close()
		log.Println("Migrations completed")
	}

	if *seed {
		log.Println("Seeding admin user + categories...")
		db, err := repository.NewDB(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("Failed to create DB connection for seeding: %v", err)
		}
		if err := db.SeedAdmin(); err != nil {
			log.Printf("Warning: seed admin: %v (maybe already exists)", err)
		} else {
			log.Println("Admin user seeded")
		}
		db.Close()
	}

	if *cleanupTokens {
		log.Println("Purging expired refresh tokens...")
		if err := refreshTokenRepo.CleanupExpired(); err != nil {
			log.Fatalf("Token cleanup failed: %v", err)
		}
		log.Println("Expired refresh tokens purged")
	}

	if *listUsers {
		users, err := userRepo.ListAll()
		if err != nil {
			log.Fatalf("Failed to list users: %v", err)
		}
		if len(users) == 0 {
			fmt.Println("No users found.")
		} else {
			fmt.Printf("%-36s %-30s %-20s\n", "ID", "EMAIL", "CREATED AT")
			fmt.Println("----------------------------------------------------" +
				"------------------------------")
			for _, u := range users {
				fmt.Printf("%-36s %-30s %-20s\n", u.ID, u.Email,
					u.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
	}

	// Jika tidak ada flag, tampilkan help
	if !*seed && !*migrate && !*cleanupTokens && !*listUsers {
		fmt.Println("Usage: go run cmd/setup/main.go [flags]")
		fmt.Println("")
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}

	os.Exit(0)
}
