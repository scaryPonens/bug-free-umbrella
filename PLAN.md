# Crypto Trading Advisor Bot — Phased Plan

> **Project:** A Go-based crypto trading advisor bot that follows crypto prices, trade volumes, and sentiment using deterministic signal generation plus an advisor layer. Recommendations are scored on a 1–5 risk spectrum. Interfaces via Telegram bot and, later, an SSH terminal UI (à la terminal.shop). Deploy target: Railway app service with NeonDB + Redis Cloud.

---


## Phase 0: Foundation & Skeleton ✅ **(Completed)**

**Goal:** Deployable Go service on Railway that does *one thing* end-to-end.

- Go module scaffolded with clean project layout (`cmd/`, `internal/`, `pkg/`)
- Telegram bot basics via [telebot](https://github.com/tucnak/telebot) — responds to `/ping`
- Postgres (local via Docker, Railway-ready) as primary store
- Redis (local via Docker, Railway-ready) for caching/rate limiting
- Config management (env vars, Railway secrets, .env file supported)
- CI: GitHub Actions → deploy to Railway (**pending**)
- Basic domain types: `Asset`, `Signal`, `Recommendation`, `RiskLevel(1-5)`

**Deliverable:**
- Bot replies to you on Telegram (`/ping` → `pong`)
- App runs locally with Docker Compose (includes Postgres, Redis, OTel, Jaeger)
- Database and Redis connections established
- Configurable via `.env` file

**Status:**
Phase 0 is complete. The project is scaffolded, all core services are wired, and local development is fully supported. CI/CD to Railway is the only remaining step for full automation.

---

## Phase 1: Price Feeds & Storage ✅ **(Completed)**

**Goal:** Ingest and persist real-time crypto price/volume data.

- CoinGecko free tier API integration (no key needed, ~10 calls/min)
- Top 10 assets tracked: BTC, ETH, SOL, XRP, ADA, DOGE, DOT, AVAX, LINK, MATIC
- OHLCV candle intervals: 5m, 15m, 1h, 4h, 1d
- 3-tier background polling (current prices every 60s, short candles every 5m, long candles every 30m)
- Token bucket rate limiter (8 tokens/min) to stay within CoinGecko limits
- Candles stored in Postgres with composite PK `(symbol, interval, open_time)` and idempotent upserts
- Redis cache-aside for latest prices (90s TTL, refreshed every 60s)
- HTTP endpoints: `GET /api/prices`, `GET /api/prices/:symbol`, `GET /api/candles/:symbol`
- Telegram commands: `/price BTC`, `/volume SOL`
- Automated CI: GitHub Actions workflow runs `go test ./...` with coverage on every push to `main`, with README badges surfacing test/coverage status
- Extensive unit test suite covering services, handlers, providers, caching, tracing, and bootstrapping logic (added in previous iteration)

**Deliverable:** Bot responds with live prices. Data accumulates in Postgres. Prices cached in Redis.

**Status:**
Phase 1 is complete. Price ingestion pipeline is fully wired — CoinGecko provider, Postgres candle storage, Redis caching, background polling, HTTP endpoints, and Telegram commands are all operational. CI now enforces the growing unit test suite (85%+ package-level coverage; ~77% total) and README badges advertise build health.

---

## Phase 2: Signal Engine v1 — Classic Indicators ✅ **(Completed)**

**Goal:** First actual trade signals using well-understood technical analysis.

- Implement as a pipeline of pure functions: `[]Candle → []Signal`
- Start with a few classics:
  - **RSI** (oversold/overbought)
  - **MACD** crossovers
  - **Bollinger Band** breakouts
  - **Volume anomaly detection** (simple z-score)
- Each signal tagged with a `RiskLevel`:
  - RSI divergence on daily = risk 2
  - MACD cross on 15m = risk 4
  - Bollinger squeeze breakout on 5m = risk 5
- Store signals in Postgres with timestamp, asset, indicator, risk, direction (long/short/hold)
- Telegram: `/signals BTC`, `/signals --risk 3` filters

**Deliverable:** Bot proactively (or on-demand) tells you "RSI on ETH is oversold on 4h, risk 2."

**Status:**
Phase 2 is complete. The repository now includes:
- A deterministic classic indicator engine (`RSI`, `MACD`, `Bollinger breakout`, `volume z-score`) implemented as pure candle-to-signal functions.
- Signal persistence in Postgres with idempotent inserts and query filters.
- Signal generation poller (short and long interval tiers) plus HTTP endpoint `GET /api/signals`.
- Telegram signal command support (`/signals BTC`, `/signals --risk 3`).
- Proactive Telegram signal alerts with chat-level opt-in/out commands (`/alerts on|off|status`).

---

## Phase 3: MCP Service Layer (Data + Tools First)

**Goal:** Expose bot capabilities through an MCP service before building the advisor UI/LLM layer.

- Stand up an MCP server in Go that exposes:
  - Price reads (`latest prices`, `price by symbol`)
  - Candle reads (`candles by symbol/interval/limit`)
  - Signal reads (`signals with symbol/risk/indicator filters`)
  - Optional signal trigger tool (`generate signals for symbol/interval`) for controlled workflows
- Treat existing HTTP/service layer as source of truth; MCP is an integration contract, not duplicated logic
- Add auth and access controls appropriate for external tool clients
- Add operational guardrails: request limits, structured errors, tracing spans around tool calls
- Keep signal generation deterministic; MCP only exposes and orchestrates existing capabilities

**Deliverable:** Any MCP-compatible client can query prices/candles/signals and invoke supported tools without using Telegram or direct DB access.

---

## Phase 4: LLM Integration — The Advisor Layer

**Goal:** Natural language interaction powered by an LLM that synthesizes signals via the MCP surface.

- Integrate Claude or OpenAI API as the "advisor brain"
- System prompt encodes your risk framework, trading philosophy, and current signals as context
- Prefer fetching data through MCP tools/resources (or equivalent service adapters) for consistency
- Flow: `user message → gather context (signals + prices) → construct prompt → LLM response → Telegram`
- Conversation history stored in Supabase (per-user thread)
- Commands become conversational: "What do you think about SOL right now?" or "Give me your riskiest play"
- The LLM doesn't *make* the signals — it **interprets and communicates** them

**Deliverable:** You chat with the bot naturally. It references real data and real signals through a stable integration layer.

---

## Phase 5: ML Signal Engine v2

**Goal:** Layer in learned models alongside classic indicators.

- **Feature store** in Postgres: pre-computed features per asset per timeframe
- Start simple:
  - **Logistic regression / XGBoost** (call out to a Python sidecar or use a Go ML lib like `golearn`) for "will price be higher in N hours?" binary classification
  - Train on your accumulated OHLCV + indicator data
- More advanced (later in this phase):
  - **LSTM or Transformer** price prediction (Python microservice, exposed via gRPC or HTTP)
  - **Anomaly detection** on volume/price patterns (isolation forest)
- Each ML model outputs a confidence score → mapped to risk level
- Ensemble: combine classic signals + ML signals with weighted voting
- A/B track: store predictions vs actuals to measure model accuracy over time

**Deliverable:** Signals now come from both rule-based and ML sources. You have a backtesting table showing accuracy.

---

## Phase 6: Fundamentals & Sentiment

**Goal:** Non-price signals.

- **On-chain fundamentals** (for crypto): whale wallet movements, exchange inflows/outflows via Glassnode or free alternatives
- **Sentiment analysis**: scrape/poll crypto Twitter (via API), Reddit (pushshift), Fear & Greed Index
  - LLM-based sentiment scoring on headlines/posts (batch job)
  - Store sentiment scores as another signal type
- **Fundamentals for tradfi crypto exposure** (COIN, MARA, etc.): earnings, P/E, sector rotation signals from Alpha Vantage
- Feed all of these into the advisor LLM context window

**Deliverable:** Bot says things like "BTC RSI is neutral but on-chain shows massive exchange outflows and sentiment is shifting bullish — risk 3 long."

---

## Phase 7: SSH Terminal Interface

**Goal:** `ssh trading@yourdomain.com` gives you a TUI dashboard.

- Use [Wish](https://github.com/charmbracelet/wish) (Charm's SSH server library for Go) + [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI
- Screens:
  - **Dashboard**: live prices, active signals, portfolio heat map
  - **Chat**: same LLM advisor, but in terminal
  - **Signal explorer**: filter by asset, risk, indicator type
  - **Backtest viewer**: model accuracy over time
- Auth via SSH keys (map public keys to users in Supabase)
- Deploy as a separate Railway service exposing port 22 (or custom port)

**Deliverable:** Full terminal.shop-style experience. You SSH in and get a live trading advisor dashboard.

---

## Phase 8: Hardening & Portfolio Tracking

- Paper trading / portfolio simulation (track hypothetical positions based on signals you "accept")
- Risk-adjusted returns tracking (Sharpe ratio per risk level)
- Alert system: proactive Telegram/SSH notifications when high-confidence signals fire
- Rate limiting, error budgets, observability (structured logging, OpenTelemetry)
- Multi-user support if you want to share it

**Status update (early work pulled forward):**
- Telegram-side proactive alerts are already partially implemented in Phase 2 (`/alerts on|off|status`), with dedupe logic to avoid repeated notifications for identical generated signals.

---

## Cross-Phase Infra Update ✅ **(Completed)**

**Goal:** Make schema evolution and deployment safer for production.

- Migration SQL moved out of repository `RunMigrations` methods into versioned SQL files.
- Dedicated migration runner added at `cmd/migrate` (`up`, `down`, `version`).
- App startup no longer mutates database schema automatically.
- Container image now includes both `main` and `migrate` binaries for deploy-time migrations.

**Status:**
Complete. Recommended deployment flow is now:
1. Run `./migrate up`
2. Start `./main`

## Architecture at a Glance

```
┌─────────────┐     ┌─────────────┐
│  Telegram    │     │  SSH/TUI    │
│  Interface   │     │  (Wish+BT)  │
└──────┬───────┘     └──────┬──────┘
       │                    │
       └────────┬───────────┘
                │
         ┌──────▼──────┐
         │  MCP Service │ ← tools/resources contract
         │  (Go service)│
         └──────┬───────┘
                │
         ┌──────▼──────┐
         │  LLM Advisor │ ← Claude/OpenAI API
         │  (consumer)  │
         └──────┬───────┘
                │
         ┌──────▼──────┐
         │ Signal Engine │
         │ ┌───────────┐│
         │ │ Classic TA ││ ← pure functions
         │ ├───────────┤│
         │ │ ML Models  ││ ← Go or Python sidecar
         │ ├───────────┤│
         │ │ Sentiment  ││ ← LLM batch scoring
         │ └───────────┘│
         └──────┬───────┘
                │
    ┌───────────┼───────────┐
    │           │           │
┌───▼──┐  ┌────▼───┐  ┌────▼───┐
│NeonDB│  │Supabase│  │ Redis  │
│(OHLCV│  │(users, │  │(cache, │
│ ts)  │  │signals)│  │ rates) │
└──────┘  └────────┘  └────────┘
```

---

## Suggested Tech Choices (Revalidated)

| Concern | Choice | Why |
|---|---|---|
| Telegram bot | `telebot` (already integrated) | Existing implementation is live and tested |
| MCP | Go MCP server | Stable integration contract before advisor clients |
| SSH TUI | Wish + Bubble Tea | Charm ecosystem, built for this |
| LLM | Claude API | Already in the ecosystem |
| TA indicators | `github.com/sdcoffey/techan` or hand-roll | Techan is decent; pure functions are easy to write in Go |
| ML sidecar | Python (FastAPI + scikit-learn/XGBoost) | Lean into Python strength for ML parts |
| Observability | Structured logging + OpenTelemetry | Production-grade from day one |

---

## Sprint Plan

| Sprint | Phases | Duration | Key Milestone |
|---|---|---|---|
| 1 | 0 + 1 | 1–2 weeks | Bot responds with live prices, data accumulating |
| 2 | 2 | 1–2 weeks | First trading signals (RSI, MACD, Bollinger) ✅ |
| 3 | 3 | 1 week | MCP service exposed for prices/candles/signals |
| 4 | 4 | 1 week | Natural language advisor uses MCP-backed context |
| 5 | 5 | 2–3 weeks | ML models running, backtesting accuracy tracked |
| 6 | 6 | 2 weeks | Sentiment + on-chain signals integrated |
| 7 | 7 | 1–2 weeks | SSH terminal interface live |
| 8 | 8 | Ongoing | Paper trading, alerts, hardening |
