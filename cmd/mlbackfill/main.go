package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/provider"
	"bug-free-umbrella/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultDays      = 90
	backfillInterval = "1h"
)

var (
	loadEnvFunc = godotenv.Load
	openPool    = pgxpool.New
)

type options struct {
	days    int
	symbols []string
}

func main() {
	loadEnvFunc()

	opts, err := parseOptions(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatalf("parse options: %v", err)
	}

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	pool, err := openPool(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	tracer := trace.NewNoopTracerProvider().Tracer("ml-backfill")
	candleRepo := repository.NewCandleRepository(pool, tracer)
	cgProvider := provider.NewCoinGeckoProvider(tracer)

	log.Printf("starting 1h candle backfill: days=%d symbols=%s", opts.days, strings.Join(opts.symbols, ","))

	totalUpserted := 0
	for _, symbol := range opts.symbols {
		candles, err := cgProvider.FetchMarketChart(ctx, symbol, opts.days, []string{backfillInterval})
		if err != nil {
			log.Fatalf("fetch market chart for %s: %v", symbol, err)
		}
		if len(candles) == 0 {
			log.Printf("no candles returned for %s", symbol)
			continue
		}
		if err := candleRepo.UpsertCandles(ctx, candles); err != nil {
			log.Fatalf("upsert candles for %s: %v", symbol, err)
		}
		totalUpserted += len(candles)
		log.Printf("backfilled %s: %d candles", symbol, len(candles))
	}

	log.Printf("backfill complete: symbols=%d total_candles=%d interval=%s days=%d", len(opts.symbols), totalUpserted, backfillInterval, opts.days)
}

func parseOptions(args []string, getenv func(string) string) (options, error) {
	fs := flag.NewFlagSet("mlbackfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	daysDefault := defaultBackfillDays(getenv)
	days := fs.Int("days", daysDefault, "number of historical days to backfill (default from ML_BACKFILL_DAYS, then ML_TRAIN_WINDOW_DAYS, else 90)")
	symbolsRaw := fs.String("symbols", strings.Join(domain.SupportedSymbols, ","), "comma-separated symbols to backfill")

	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if *days <= 0 {
		return options{}, fmt.Errorf("days must be > 0")
	}

	symbols, err := normalizeSymbols(*symbolsRaw)
	if err != nil {
		return options{}, err
	}

	return options{
		days:    *days,
		symbols: symbols,
	}, nil
}

func defaultBackfillDays(getenv func(string) string) int {
	for _, key := range []string{"ML_BACKFILL_DAYS", "ML_TRAIN_WINDOW_DAYS"} {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			continue
		}
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return defaultDays
}

func normalizeSymbols(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("symbols cannot be empty")
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.ToUpper(strings.TrimSpace(p))
		if s == "" {
			continue
		}
		if _, ok := domain.CoinGeckoID[s]; !ok {
			return nil, fmt.Errorf("unsupported symbol: %s", s)
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("symbols cannot be empty")
	}
	return out, nil
}
