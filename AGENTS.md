# Codex Project Context: bug-free-umbrella

This file is the default repo context for Codex sessions in this project.

## Project Overview

`bug-free-umbrella` is a Go-based crypto trading advisor bot. It ingests market and sentiment data, generates classic + ML signals, and exposes capabilities through:

- HTTP API (Gin + Swagger)
- Telegram bot
- MCP service (`stdio` + HTTP)
- SSH terminal interface (Wish + Bubble Tea)

Core infrastructure:
- Postgres for persistence
- Redis for cache/state
- OpenTelemetry + Jaeger for tracing/observability

## Status

- Phases 0-7 are complete.
- Phase 8 (SSH TUI experience) is planned and partially scaffolded (`cmd/ssh` exists).
- Use `PLAN.md` as the source of truth for phase scope, status notes, and rollout details.

Before implementing major work, check `PLAN.md` for intent and constraints.

## Architecture (High-Level)

```text
cmd/server/            Main API entrypoint + dependency wiring
cmd/mcp/               MCP server binary (stdio + HTTP transports)
cmd/migrate/           SQL migration runner (up/down/version)
cmd/mlbackfill/        Historical candle backfill utility
cmd/ssh/               SSH TUI entrypoint (Wish/Bubble Tea)

internal/advisor/      LLM advisor orchestration + prompt construction
internal/bot/          Telegram command handlers + alerting
internal/cache/        Redis setup
internal/chart/        Signal image rendering
internal/config/       Environment/config loading
internal/db/           Postgres setup
internal/domain/       Shared domain types
internal/handler/      HTTP handlers (Swagger-annotated)
internal/job/          Background jobs/pollers
internal/marketintel/  Fundamentals/sentiment ingest + scoring + composite signals
internal/mcp/          MCP tools/resources/auth/middleware
internal/ml/           ML features/models/training/inference/ensemble
internal/provider/     External data providers + rate limiting
internal/repository/   Data-access layer (Postgres repositories)
internal/service/      Business orchestration services
internal/signal/       Classic TA signal engine
internal/ta/           Shared TA indicator helpers
pkg/tracing/           OpenTelemetry setup
docs/                  Generated Swagger artifacts
```

## Working Conventions

- Prefer explicit dependency injection; avoid globals and hidden side effects.
- Keep DB access in repositories; services should not issue raw DB queries directly.
- Keep technical-signal computation deterministic and testable (`[]Candle -> []Signal` style where applicable).
- Preserve idempotency for ingest/write paths (upsert semantics where expected).
- Use migration SQL in `cmd/migrate/migrations`; do not modify schema ad hoc in application code.
- Treat non-critical rendering/enrichment failures as non-blocking when existing flows already do so.

## Local Development

Setup:

```bash
cp .env.example .env
docker compose --env-file .env run --rm app ./migrate up
docker compose --env-file .env up --build
```

Useful commands:

```bash
# Tests
go test ./...

# Coverage
go test ./... -coverprofile=coverage.out

# Local migrations
go run ./cmd/migrate up
go run ./cmd/migrate down

# Regenerate Swagger docs after handler annotation changes
swag init -g cmd/server/main.go

# Run MCP locally (stdio)
MCP_TRANSPORT=stdio go run ./cmd/mcp

# Backfill ML candles
go run ./cmd/mlbackfill
```

## Key Environment Variables

- `DATABASE_URL`
- `REDIS_URL`
- `TELEGRAM_BOT_TOKEN`
- `OPENAI_API_KEY` (advisor optional if unset)
- `MCP_AUTH_TOKEN` (for MCP HTTP transport)
- `ML_ENABLED`
- `MARKET_INTEL_ENABLED`

## Notes for Agents

- Keep changes small and composable; prefer targeted diffs over broad rewrites.
- Update tests/docs when behavior changes.
- Do not edit generated files manually unless intentionally regenerating them (`docs/` for Swagger output).
- If uncertain about behavior, inspect existing tests first; this repo has strong test coverage in most packages.
