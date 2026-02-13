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
	defaultDays = 90
)

var (
	loadEnvFunc = godotenv.Load
	openPool    = pgxpool.New
)

type options struct {
	days      int
	symbols   []string
	intervals []string
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

	log.Printf(
		"starting candle backfill: days=%d symbols=%s intervals=%s",
		opts.days,
		strings.Join(opts.symbols, ","),
		strings.Join(opts.intervals, ","),
	)

	totalUpserted := 0
	for _, symbol := range opts.symbols {
		candles, err := cgProvider.FetchMarketChart(ctx, symbol, opts.days, opts.intervals)
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

	log.Printf(
		"backfill complete: symbols=%d total_candles=%d intervals=%s days=%d",
		len(opts.symbols),
		totalUpserted,
		strings.Join(opts.intervals, ","),
		opts.days,
	)
}

func parseOptions(args []string, getenv func(string) string) (options, error) {
	fs := flag.NewFlagSet("mlbackfill", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	daysDefault := defaultBackfillDays(getenv)
	intervalsDefault := defaultBackfillIntervals(getenv)
	days := fs.Int("days", daysDefault, "number of historical days to backfill (default from ML_BACKFILL_DAYS, then ML_TRAIN_WINDOW_DAYS, else 90)")
	symbolsRaw := fs.String("symbols", strings.Join(domain.SupportedSymbols, ","), "comma-separated symbols to backfill")
	intervalsRaw := fs.String("intervals", strings.Join(intervalsDefault, ","), "comma-separated candle intervals to backfill")

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
	intervals, err := normalizeIntervals(*intervalsRaw)
	if err != nil {
		return options{}, err
	}

	return options{
		days:      *days,
		symbols:   symbols,
		intervals: intervals,
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

func defaultBackfillIntervals(getenv func(string) string) []string {
	candidates := []string{
		strings.TrimSpace(getenv("ML_INTERVALS")),
		strings.TrimSpace(getenv("ML_INTERVAL")),
		"1h",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		intervals, err := normalizeIntervals(candidate)
		if err == nil && len(intervals) > 0 {
			return intervals
		}
	}
	return []string{"1h"}
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

func normalizeIntervals(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("intervals cannot be empty")
	}
	allowed := make(map[string]struct{}, len(domain.SupportedIntervals))
	for _, interval := range domain.SupportedIntervals {
		allowed[interval] = struct{}{}
	}
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		interval := strings.TrimSpace(part)
		if interval == "" {
			continue
		}
		if _, ok := allowed[interval]; !ok {
			return nil, fmt.Errorf("unsupported interval: %s", interval)
		}
		if _, exists := seen[interval]; exists {
			continue
		}
		seen[interval] = struct{}{}
		out = append(out, interval)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("intervals cannot be empty")
	}
	return out, nil
}
