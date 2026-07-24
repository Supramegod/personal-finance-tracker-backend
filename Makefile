# ============================================
# Personal Finance Tracker — Backend Makefile
# ============================================

.PHONY: run build test test-cover test-short test-integration migrate swagger clean help \
        docker-up docker-down docker-reset docker-build docker-run \
        k8s-apply k8s-status k8s-logs k8s-delete

# ─── Variables ─────────────────────────────────
APP_NAME    := personal-finance-tracker
# MAIN_PKG dipakai untuk build/run — harus package (./cmd/server), bukan file
# tunggal, karena package main tersebar di main.go, cleanup.go, dan probe.go.
MAIN_PKG    := ./cmd/server
# MAIN_PATH hanya untuk swag init yang memang butuh path file entry point.
MAIN_PATH   := cmd/server/main.go
BUILD_DIR   := build
COVERAGE_DIR := coverage
IMAGE       := personal-finance-tracker-api:0.1.0
K8S_NAMESPACE := finance-tracker

# ─── Help ──────────────────────────────────────
help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Development ────────────────────────────────
run: ## Start the server (jalankan dari folder backend/ — migrations pakai path relatif db/migrations)
	go run $(MAIN_PKG)

build: ## Build binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

# ─── Testing ──────────────────────────────────
test: ## Run all tests
	go test ./... -v -count=1

test-cover: ## Run tests with coverage
	@mkdir -p $(COVERAGE_DIR)
	go test ./... -coverprofile=$(COVERAGE_DIR)/cover.out -count=1
	go tool cover -html=$(COVERAGE_DIR)/cover.out -o $(COVERAGE_DIR)/cover.html
	@echo "Coverage report: $(COVERAGE_DIR)/cover.html"

test-short: ## Run tests without integration (DB-dependent) tests
	go test ./pkg/... -v -count=1 -short

test-service: ## Run service tests (requires PostgreSQL)
	go test ./test/service/... -v -count=1

test-integration: ## Run integration tests (requires DB + server running)
	RUN_INTEGRATION_TESTS=1 go test ./test/integration/... -v -count=1

# ─── Database ──────────────────────────────────
migrate: ## Run database migrations (via Go app)
	@echo "Migrations are auto-run on server start"
	@echo "Run 'make run' to start the server with migrations"

# ─── Swagger ────────────────────────────────────
swagger: ## Regenerate Swagger documentation
	swag init -g cmd/server/main.go --output docs

# ─── Cleanup ──────────────────────────────────
clean: ## Clean build artifacts and coverage
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)

# ─── Docker ─────────────────────────────────────
docker-up: ## Start PostgreSQL via Docker Compose
	docker compose -f ../docker-compose.yml up -d

docker-down: ## Stop PostgreSQL
	docker compose -f ../docker-compose.yml down

docker-reset: ## Reset database (WARNING: deletes all data!)
	docker compose -f ../docker-compose.yml down -v
	docker compose -f ../docker-compose.yml up -d

docker-build: ## Build Docker image for the Go API
	docker build -t $(IMAGE) -f Dockerfile .

docker-run: docker-build ## Build and run the API in a container
	docker run -p 8080:8080 --env-file .env $(IMAGE)

# ─── Kubernetes ─────────────────────────────────
k8s-apply: ## Apply semua manifest (Secret harus dibuat lebih dulu — lihat k8s/README.md)
	kubectl apply -k k8s/

k8s-status: ## Lihat status rollout dan pod
	kubectl -n $(K8S_NAMESPACE) get pods
	kubectl -n $(K8S_NAMESPACE) rollout status deploy/finance-api

k8s-logs: ## Ikuti log API
	kubectl -n $(K8S_NAMESPACE) logs -f deploy/finance-api

k8s-delete: ## Hapus semua resource KECUALI Secret dan PVC (data DB tetap aman)
	kubectl delete -k k8s/
