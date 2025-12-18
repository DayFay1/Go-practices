# Polling / Voting System (Go)

JSON REST API for polls creation and vote casting. The stack is Go + chi + Postgres, with JWT authentication, worker pool, metrics, and Docker support.

## Requirements

- Go (64-bit, recommended 1.22+)
- Docker + Docker Compose

## Project structure

- `cmd/server` - entrypoint
- `internal/config` - env/config loading
- `internal/domain` – domain models and services
- `internal/repository/postgres` – Postgres repositories
- `internal/http` – router, handlers, middleware
- `internal/worker` – vote aggregation worker pool
- `internal/metrics` – Prometheus counters
- `internal/db/migrations` – SQL migrations
- `docs` - Swagger docs

## Quickstart (Docker Compose)

This repo ships with `docker-compose.yml` that runs:
- `db` - Postgres (user/pass: `polling_user` / `polling_pass`, db: `polling_db`)
- `api` - the Go HTTP server on `http://localhost:8080`
- `migrate` - a one-off utility container (golang-migrate) to run DB migrations

Before running, create `.env` from `.env.example` (at minimum `JWT_SECRET` is required).

### First start (fresh DB)

1) Start Postgres:

```bash
docker compose up -d db
```

2) Apply migrations:

```bash
docker compose run --rm migrate up
```

3) Start API:

```bash
docker compose up -d --build api
```

4) Verify:
- Health: `curl http://localhost:8080/health`
- Readiness (DB): `curl http://localhost:8080/ready`
- Swagger UI: `http://localhost:8080/swagger/index.html`
- Metrics: `http://localhost:8080/metrics`

### Next runs (DB already migrated)

```bash
docker compose up -d --build
```

### Stop / cleanup

- Stop containers (keeps DB data): `docker compose stop`
- Stop + remove containers (keeps DB data): `docker compose down`
- Full reset (drops DB volume!): `docker compose down -v`

### Environment

The API reads `.env` (or container env vars). You can start by copying `.env.example` to `.env`.
- `APP_PORT` (default `8080`)
- `DB_DSN` (compose default: `postgres://polling_user:polling_pass@db:5432/polling_db?sslmode=disable`)
- `JWT_SECRET` (required)
- `JWT_ISSUER` (default `polling-system`)
- `TRUSTED_PROXIES` (optional, comma-separated CIDRs; enables trusting `X-Forwarded-For` only from those proxies)

## Migrations (golang-migrate CLI)

Via compose (recommended):
- Up: `docker compose run --rm migrate up`
- Down 1 step: `docker compose run --rm migrate down 1`
- Version: `docker compose run --rm migrate version`

Via local `migrate` binary:

```bash
migrate -path internal/db/migrations -database "postgres://polling_user:polling_pass@localhost:5432/polling_db?sslmode=disable" up
```

Tables: `users`, `polls`, `options`, `votes`, `aggregated_results`, indexes, and a seeded admin (`admin@example.com`).

## Run the API locally

```bash
export APP_PORT=8080
export DB_DSN="postgres://polling_user:polling_pass@localhost:5432/polling_db?sslmode=disable"
export JWT_SECRET="super-secret-change-me"
export JWT_ISSUER="polling-system"
go run ./cmd/server
```

Health check: `curl http://localhost:8080/health`
Readiness: `curl http://localhost:8080/ready`

Swagger UI: `http://localhost:8080/swagger/index.html`

Prometheus metrics: `http://localhost:8080/metrics`

## Docker (manual build/run)

Build and run the app container (expects the `db` compose service):

```bash
docker build -t polling-system .
docker run --rm -p 8080:8080 --env DB_DSN="postgres://polling_user:polling_pass@db:5432/polling_db?sslmode=disable" --env JWT_SECRET="super-secret-change-me" polling-system
```

## Testing

```bash
go test ./...
```

## API overview

Public:
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`

Authenticated:
- `GET  /api/v1/polls`
- `GET  /api/v1/polls/{id}`
- `POST /api/v1/polls/{id}/vote`
- `GET  /api/v1/polls/{id}/results`
- `GET  /health`
- `GET  /ready`

Admin-only:
- `POST  /api/v1/polls`
- `PATCH /api/v1/polls/{id}`
- `PATCH /api/v1/polls/{id}/status`
- `DELETE /api/v1/polls/{id}`
- `GET   /api/v1/users`
- `PATCH /api/v1/users/{id}/role`
- `PATCH /api/v1/users/{id}/deactivate`

## Error format

All errors are JSON:

```json
{"error": "invalid_input", "message": "title is required"}
```

Status mapping:
- `400` – validation / bad input
- `401` – invalid token / credentials / inactive user
- `403` – RBAC failures
- `404` – entity not found
- `409` – conflicts (e.g., duplicate vote)
- `500/503` – unexpected / dependency unavailable

## Behavior and reliability

- JWT auth with roles (`admin`, `user`), bcrypt password hashing.
- Inactive users are rejected at login (`is_active=false`).
- Voting is idempotent per poll/user via DB unique constraint; duplicate votes return HTTP 409.
- Poll must be `active` to accept votes; `draft`/`closed` votes are rejected.
- Options are validated against the poll by composite FK and service errors.
- In-memory cache (10s TTL) for poll results with invalidation on new votes.
- Rate limiting on the vote endpoint (per-IP limiter) plus CORS and structured request logging.
- Worker pool consumes vote events and updates aggregated results with retry + backoff.
- Prometheus counter `polling_http_requests_total` (method/path/status) exposed at `/metrics`.
- Graceful shutdown handles SIGINT/SIGTERM, closes the vote channel, and waits for the worker pool to drain (with timeout).

## Example curl calls

```bash
# Login (admin)
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"<your-admin-password>"}'

# Create a poll (admin token)
curl -X POST http://localhost:8080/api/v1/polls \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Lunch?","options":["Pizza","Salad"]}'

# Vote (user token)
curl -X POST http://localhost:8080/api/v1/polls/1/vote \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"option_id":2}'
```
