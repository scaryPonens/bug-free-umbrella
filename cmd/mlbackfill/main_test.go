package main

import (
	"reflect"
	"testing"
)

func TestDefaultBackfillDays(t *testing.T) {
	getenv := func(key string) string { return "" }
	if got := defaultBackfillDays(getenv); got != defaultDays {
		t.Fatalf("expected default %d, got %d", defaultDays, got)
	}

	getenv = func(key string) string {
		if key == "ML_TRAIN_WINDOW_DAYS" {
			return "120"
		}
		return ""
	}
	if got := defaultBackfillDays(getenv); got != 120 {
		t.Fatalf("expected 120, got %d", got)
	}

	getenv = func(key string) string {
		if key == "ML_BACKFILL_DAYS" {
			return "45"
		}
		if key == "ML_TRAIN_WINDOW_DAYS" {
			return "120"
		}
		return ""
	}
	if got := defaultBackfillDays(getenv); got != 45 {
		t.Fatalf("expected ML_BACKFILL_DAYS precedence, got %d", got)
	}
}

func TestNormalizeSymbols(t *testing.T) {
	symbols, err := normalizeSymbols("btc, ETH,btc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"BTC", "ETH"}
	if !reflect.DeepEqual(symbols, expected) {
		t.Fatalf("expected %v, got %v", expected, symbols)
	}

	if _, err := normalizeSymbols("FAKE"); err == nil {
		t.Fatal("expected unsupported symbol error")
	}

	if _, err := normalizeSymbols(" ,, "); err == nil {
		t.Fatal("expected empty symbol error")
	}
}

func TestParseOptions(t *testing.T) {
	getenv := func(key string) string {
		if key == "ML_TRAIN_WINDOW_DAYS" {
			return "75"
		}
		return ""
	}

	opts, err := parseOptions([]string{"--symbols", "BTC,ETH"}, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.days != 75 {
		t.Fatalf("expected days=75 from env, got %d", opts.days)
	}
	if !reflect.DeepEqual(opts.symbols, []string{"BTC", "ETH"}) {
		t.Fatalf("unexpected symbols: %v", opts.symbols)
	}

	opts, err = parseOptions([]string{"--days", "30", "--symbols", "BTC"}, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.days != 30 {
		t.Fatalf("expected days=30, got %d", opts.days)
	}

	if _, err := parseOptions([]string{"--days", "0"}, getenv); err == nil {
		t.Fatal("expected invalid days error")
	}
}
