# ============================================
# Stage 1: Build
# ============================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Leverage Docker cache: copy go.mod and go.sum first
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary.
# WAJIB build seluruh package ./cmd/server, bukan cmd/server/main.go saja —
# package main tersebar di main.go, cleanup.go, dan probe.go. Build per-file
# akan gagal dengan "undefined: StartTokenCleanup".
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-w -s" -o /app/server ./cmd/server

# Binary CLI operasional: -migrate, -seed, -cleanup-tokens, -list-users.
# Dipakai lewat `kubectl exec` untuk tugas manual.
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-w -s" -o /app/setup ./cmd/setup

# ============================================
# Stage 2: Runtime
# ============================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget

# Set timezone ke WIB (UTC+7)
ENV TZ=Asia/Jakarta

# Jalankan sebagai non-root
RUN adduser -D -u 65532 -h /app appuser

WORKDIR /app

# Copy binary dan folder migrations.
# Migrations WAJIB ada di ./db/migrations relatif terhadap working directory —
# repository.RunMigrations() memakai path relatif, bukan embed.
COPY --from=builder /app/server ./server
COPY --from=builder /app/setup ./setup
COPY --from=builder /app/db/migrations ./db/migrations

USER appuser

EXPOSE 8080

# Health check untuk pemakaian docker/compose. Di Kubernetes probe diatur
# lewat manifest (livenessProbe/readinessProbe), bukan dari sini.
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health || exit 1

CMD ["./server"]
