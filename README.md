# bug-free-umbrella


Go-based crypto trading advisor bot foundation. Includes:
- [Gin](https://github.com/gin-gonic/gin) web API
- Telegram bot (responds to `/ping`)
- Postgres & Redis integration (via Docker Compose)
- OpenTelemetry tracing, Jaeger, Swagger docs
- Configurable via `.env` file

## Stack

- **Go / Gin** - HTTP framework
- **OpenTelemetry** - Distributed tracing (exported via gRPC to an OTel Collector)
- **Jaeger** - Trace visualization
- **Swag** - Auto-generated OpenAPI/Swagger docs from annotations

## Project Structure

```
cmd/server/          Entrypoint and dependency wiring
internal/handler/    HTTP handlers with Swagger annotations
internal/service/    Business logic
pkg/tracing/         OpenTelemetry initialization
docs/                Generated Swagger spec (do not edit manually)
```


## Setup & Running

1. Copy `.env.example` to `.env` and fill in required secrets (see below).
2. Start all services (API, Postgres, Redis, OTel, Jaeger) with:

```sh
docker compose --env-file .env up --build
```

The app will be available at http://localhost:8080

### .env file

Example:

```env
TRACING_ENABLED=false
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
GIN_MODE=debug

# Telegram Bot
TELEGRAM_BOT_TOKEN=your-telegram-bot-token

# Postgres Database
DATABASE_URL=postgres://postgres:postgres@db:5432/postgres?sslmode=disable

# Redis
REDIS_URL=redis:6379
```

> **Note:** The default Docker Compose setup will run Postgres and Redis containers for you. The app will auto-connect using the above variables.

| Service     | URL                                    |
|-------------|----------------------------------------|
| API         | http://localhost:8080                  |
| Swagger UI  | http://localhost:8080/swagger/index.html |
| Jaeger UI   | http://localhost:16686                 |
| Postgres    | localhost:5432 (inside Docker: db:5432) |
| Redis       | localhost:6379 (inside Docker: redis:6379) |


## API Endpoints

| Method | Path         | Description                        |
|--------|--------------|------------------------------------|
| GET    | /health      | Health check                       |
| GET    | /api/hello   | Hello world greeting               |
| GET    | /api/slow    | Simulated slow response (150ms)    |

## Telegram Bot

The bot will start automatically if `TELEGRAM_BOT_TOKEN` is set in your `.env` file. It responds to `/ping` with `pong`.


## Regenerating Swagger Docs

After adding or modifying handler annotations:

```sh
swag init -g cmd/server/main.go
```

This runs automatically during the Docker build.
