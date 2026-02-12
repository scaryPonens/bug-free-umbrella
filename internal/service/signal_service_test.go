package service

import (
	"context"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

func TestSignalServiceGenerateForSymbolUnsupported(t *testing.T) {
	svc := NewSignalService(
		trace.NewNoopTracerProvider().Tracer("test"),
		&stubSignalCandleRepo{},
		&stubSignalRepo{},
		&stubSignalEngine{},
	)

	if _, err := svc.GenerateForSymbol(context.Background(), "FAKE", nil); err == nil {
		t.Fatal("expected unsupported symbol error")
	}
}

func TestSignalServiceGenerateForSymbolPersistsGeneratedSignals(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	candleRepo := &stubSignalCandleRepo{
		candles: map[string][]*domain.Candle{
			"1h": {{
				Symbol:   "BTC",
				Interval: "1h",
				OpenTime: time.Now().UTC(),
				Close:    101,
				Volume:   10,
			}},
		},
	}
	signalRepo := &stubSignalRepo{}
	engine := &stubSignalEngine{
		signals: []domain.Signal{{
			Symbol:    "BTC",
			Interval:  "1h",
			Indicator: domain.IndicatorRSI,
			Direction: domain.DirectionLong,
			Risk:      domain.RiskLevel3,
			Timestamp: time.Unix(0, 0).UTC(),
		}},
	}
	svc := NewSignalService(tracer, candleRepo, signalRepo, engine)

	got, err := svc.GenerateForSymbol(context.Background(), "btc", []string{"1h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	if signalRepo.insertCalls != 1 {
		t.Fatalf("expected insert call, got %d", signalRepo.insertCalls)
	}
	if candleRepo.lastSymbol != "BTC" || candleRepo.lastInterval != "1h" {
		t.Fatalf("unexpected candle query args: %s %s", candleRepo.lastSymbol, candleRepo.lastInterval)
	}
}

func TestSignalServiceListSignalsValidatesFilter(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	signalRepo := &stubSignalRepo{}
	svc := NewSignalService(tracer, &stubSignalCandleRepo{}, signalRepo, &stubSignalEngine{})

	invalid := domain.RiskLevel(6)
	if _, err := svc.ListSignals(context.Background(), domain.SignalFilter{Risk: &invalid}); err == nil {
		t.Fatal("expected invalid risk error")
	}

	risk := domain.RiskLevel3
	_, err := svc.ListSignals(context.Background(), domain.SignalFilter{
		Symbol:    "btc",
		Indicator: "MACD",
		Risk:      &risk,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signalRepo.lastFilter.Symbol != "BTC" {
		t.Fatalf("expected uppercase symbol, got %s", signalRepo.lastFilter.Symbol)
	}
	if signalRepo.lastFilter.Indicator != "macd" {
		t.Fatalf("expected lowercase indicator, got %s", signalRepo.lastFilter.Indicator)
	}
	if signalRepo.lastFilter.Limit != 50 {
		t.Fatalf("expected default limit=50, got %d", signalRepo.lastFilter.Limit)
	}
}

type stubSignalCandleRepo struct {
	candles      map[string][]*domain.Candle
	lastSymbol   string
	lastInterval string
	lastLimit    int
}

func (s *stubSignalCandleRepo) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*domain.Candle, error) {
	s.lastSymbol = symbol
	s.lastInterval = interval
	s.lastLimit = limit
	if s.candles == nil {
		return nil, nil
	}
	return s.candles[interval], nil
}

type stubSignalRepo struct {
	insertCalls int
	inserted    []domain.Signal
	lastFilter  domain.SignalFilter
	listResp    []domain.Signal
}

func (s *stubSignalRepo) InsertSignals(ctx context.Context, signals []domain.Signal) error {
	s.insertCalls++
	s.inserted = append([]domain.Signal(nil), signals...)
	return nil
}

func (s *stubSignalRepo) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.lastFilter = filter
	return append([]domain.Signal(nil), s.listResp...), nil
}

type stubSignalEngine struct {
	signals []domain.Signal
}

func (s *stubSignalEngine) Generate(candles []*domain.Candle) []domain.Signal {
	return append([]domain.Signal(nil), s.signals...)
}
