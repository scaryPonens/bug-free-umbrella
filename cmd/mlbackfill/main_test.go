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
	if !reflect.DeepEqual(opts.intervals, []string{"1h"}) {
		t.Fatalf("expected default intervals [1h], got %v", opts.intervals)
	}

	opts, err = parseOptions([]string{"--days", "30", "--symbols", "BTC", "--intervals", "1h,4h"}, getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.days != 30 {
		t.Fatalf("expected days=30, got %d", opts.days)
	}
	if !reflect.DeepEqual(opts.intervals, []string{"1h", "4h"}) {
		t.Fatalf("unexpected intervals: %v", opts.intervals)
	}

	if _, err := parseOptions([]string{"--days", "0"}, getenv); err == nil {
		t.Fatal("expected invalid days error")
	}
	if _, err := parseOptions([]string{"--intervals", "10m"}, getenv); err == nil {
		t.Fatal("expected invalid intervals error")
	}
}

func TestDefaultBackfillIntervals(t *testing.T) {
	getenv := func(key string) string { return "" }
	if got := defaultBackfillIntervals(getenv); !reflect.DeepEqual(got, []string{"1h"}) {
		t.Fatalf("expected default [1h], got %v", got)
	}

	getenv = func(key string) string {
		if key == "ML_INTERVALS" {
			return "1h,4h"
		}
		return ""
	}
	if got := defaultBackfillIntervals(getenv); !reflect.DeepEqual(got, []string{"1h", "4h"}) {
		t.Fatalf("expected [1h 4h], got %v", got)
	}

	getenv = func(key string) string {
		if key == "ML_INTERVALS" {
			return "bad"
		}
		if key == "ML_INTERVAL" {
			return "4h"
		}
		return ""
	}
	if got := defaultBackfillIntervals(getenv); !reflect.DeepEqual(got, []string{"4h"}) {
		t.Fatalf("expected fallback [4h], got %v", got)
	}
}
