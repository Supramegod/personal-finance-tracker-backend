# CLAUDE.md

Personal Finance Tracker — Go backend (Fiber + PostgreSQL).
All commands below come from the `Makefile`. Run `make help` for the full list.

## Build

```bash
make build          # go build -o build/personal-finance-tracker ./cmd/server
make clean          # remove build/ and coverage/
```

## Run

```bash
make run            # go run ./cmd/server
```

Run `make run` from the backend folder — migrations resolve `db/migrations` via a
relative path and will fail from anywhere else. Migrations auto-run on server
start; `make migrate` only prints that reminder, it does not migrate on its own.

## Test

```bash
make test              # go test ./... -v -count=1   (all tests)
make test-short        # go test ./pkg/... -short    (no DB needed)
make test-cover        # coverage → coverage/cover.html
make test-service      # ./test/service/...          (requires PostgreSQL)
make test-integration  # ./test/integration/...      (requires DB + running server)
```

`test-integration` is gated behind `RUN_INTEGRATION_TESTS=1`, which the target
sets for you. Prefer `make test-short` for a quick loop with no infrastructure.

## Lint / vet

No lint target is defined in the Makefile. Use the Go defaults:

```bash
go vet ./...
gofmt -l .
```

## Codegen (sqlc)

The repository layer is generated, not hand-written. `db/sqlc.yaml` drives it:

- schema: `db/migrations/`
- queries: `db/queries/`
- output: `internal/repository/sqlc` (package `repository`, `sql_package: pgx/v5`)

Paths inside `sqlc.yaml` are repo-root relative, so regenerate from the repo root:

```bash
sqlc generate -f db/sqlc.yaml
```

There is no `make` target for this and no CI step that checks it — after editing
anything in `db/migrations/` or `db/queries/`, regenerate by hand or the Go code
and the schema drift apart silently.

## Docker

```bash
make docker-up       # PostgreSQL only — run the API with `make run`
make docker-up-all   # PostgreSQL + API, both in containers
make docker-down     # stop everything
make docker-reset    # WARNING: drops volumes, all DB data is lost
make docker-build    # build image personal-finance-tracker-api:0.1.0
make docker-run      # build + run the API container on :8080 using .env
```

## Kubernetes

```bash
make k8s-apply    # kubectl apply -k k8s/  — create the Secret first, see k8s/README.md
make k8s-status   # pods + rollout status in namespace finance-tracker
make k8s-logs     # follow logs for deploy/finance-api
make k8s-delete   # deletes resources EXCEPT Secret and PVC (DB data is preserved)
```

## Swagger

```bash
make swagger      # swag init -g cmd/server/main.go --output docs
```

Regenerate after changing handler annotations; `docs/` is generated output.

## Notes

- Entry point package is `./cmd/server` — `package main` is split across
  `main.go`, `cleanup.go`, and `probe.go`, so build and run must target the
  package, not a single file. Only `swag init` takes the file path.
- Config comes from `.env` (see `.env.example`). `.env` is not committed.
- CI (`.github/workflows/deploy.yml`) gates on `go build ./...`, `go vet ./...`,
  and `go test ./pkg/... -short` — then builds and pushes the Docker image.
- The AI insights feature (`internal/service/ai_insight_service.go`) calls
  **Google Gemini**, not Anthropic — `GEMINI_API_KEY`, `AI_MODEL=gemini-2.5-flash-lite`.
- `internal/isapi/` is an empty placeholder (`.gitkeep` only).

## ECC surface for this repo

ECC is installed globally, so nothing lives under `./.claude/` and nothing needs
installing. `.claude/` is **not** in `.gitignore` — do not run a project-scoped
ECC install here or you will commit hundreds of vendored files.

Relevant to this stack (Go + PostgreSQL/sqlc + Docker + k8s):

| Command | Use |
|---|---|
| `/ecc:go-review` | Idiomatic Go, concurrency, error handling |
| `/ecc:go-build` | Build or `go vet` failures |
| `/ecc:go-test` | Table-driven tests first, then implementation |
| `/ecc:code-review` | General diff/PR review |
| `/ecc:security-scan` | JWT + bcrypt + API keys + k8s secrets live here |
| `/ecc:plan` | Before larger features |
| `/ecc:pr` | Open a PR from the current branch |

Skills worth reaching for: `golang-patterns`, `golang-testing`, `postgres-patterns`,
`database-migrations`, `api-design`, `error-handling`, `docker-patterns`,
`kubernetes-patterns`, `deployment-patterns`, `github-ops`.

Everything else in ECC (frontend, Python/JVM/Swift, Redis, e2e browser testing,
`claude-api`) is off-stack for this repo — still searchable, just not relevant.
