package service

import (
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
)

func TestExtractOpenAndTargetClose(t *testing.T) {
	open := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	target := open.Add(4 * time.Hour)
	candles := []*domain.Candle{
		{OpenTime: target, Close: 120},
		{OpenTime: open, Close: 100},
		{OpenTime: open.Add(2 * time.Hour), Close: 110},
	}
	openClose, targetClose, ok := extractOpenAndTargetClose(candles, open, target)
	if !ok {
		t.Fatal("expected to find open and target candles")
	}
	if openClose != 100 || targetClose != 120 {
		t.Fatalf("unexpected close values open=%.2f target=%.2f", openClose, targetClose)
	}
}

func TestUniqueIntervals(t *testing.T) {
	got := uniqueIntervals([]string{"1h", "4h", "1h"}, "1h")
	if len(got) != 2 || got[0] != "1h" || got[1] != "4h" {
		t.Fatalf("unexpected unique intervals: %v", got)
	}

	got = uniqueIntervals(nil, "1h")
	if len(got) != 1 || got[0] != "1h" {
		t.Fatalf("expected fallback interval [1h], got %v", got)
	}
}

func TestCandleLimitForInterval(t *testing.T) {
	hourly := candleLimitForInterval("1h", 90, 4)
	fourHour := candleLimitForInterval("4h", 90, 4)
	if hourly <= fourHour {
		t.Fatalf("expected 1h limit > 4h limit, got hourly=%d four_hour=%d", hourly, fourHour)
	}
	if fourHour < 500 {
		t.Fatalf("expected minimum limit floor applied, got %d", fourHour)
	}
}

func TestShouldResolvePrediction(t *testing.T) {
	if shouldResolvePrediction("iforest_1h") {
		t.Fatal("iforest predictions should be skipped by resolver")
	}
	if !shouldResolvePrediction("logreg") {
		t.Fatal("directional predictions should be resolved")
	}
}
