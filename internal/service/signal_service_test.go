package service

import (
	"context"
	"errors"
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

func TestSignalServiceGenerateForSymbolImageFailureIsNonBlocking(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	candleRepo := &stubSignalCandleRepo{
		candles: map[string][]*domain.Candle{
			"1h": {{
				Symbol:   "BTC",
				Interval: "1h",
				OpenTime: time.Now().UTC(),
				Open:     100,
				High:     110,
				Low:      90,
				Close:    105,
				Volume:   1000,
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
			Timestamp: time.Now().UTC(),
		}},
	}
	imageRepo := &stubSignalImageRepo{}
	renderer := &stubSignalChartRenderer{err: errors.New("render failed")}
	svc := NewSignalServiceWithImages(tracer, candleRepo, signalRepo, engine, imageRepo, renderer)

	got, err := svc.GenerateForSymbol(context.Background(), "BTC", []string{"1h"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(got))
	}
	if imageRepo.failureCalls != 1 {
		t.Fatalf("expected one image failure record, got %d", imageRepo.failureCalls)
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

func (s *stubSignalRepo) InsertSignals(ctx context.Context, signals []domain.Signal) ([]domain.Signal, error) {
	s.insertCalls++
	s.inserted = append([]domain.Signal(nil), signals...)
	out := append([]domain.Signal(nil), signals...)
	for i := range out {
		out[i].ID = int64(i + 1)
	}
	return out, nil
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

type stubSignalImageRepo struct {
	failureCalls int
	imageByID    map[int64]*domain.SignalImageData
}

func (s *stubSignalImageRepo) UpsertSignalImageReady(
	ctx context.Context,
	signalID int64,
	imageBytes []byte,
	mimeType string,
	width, height int,
	expiresAt time.Time,
) (*domain.SignalImageRef, error) {
	return &domain.SignalImageRef{
		ImageID:   signalID + 1000,
		MimeType:  mimeType,
		Width:     width,
		Height:    height,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *stubSignalImageRepo) UpsertSignalImageFailure(
	ctx context.Context,
	signalID int64,
	errorText string,
	nextRetryAt time.Time,
	expiresAt time.Time,
) error {
	s.failureCalls++
	return nil
}

func (s *stubSignalImageRepo) GetSignalImageBySignalID(ctx context.Context, signalID int64) (*domain.SignalImageData, error) {
	if s.imageByID == nil {
		return nil, nil
	}
	return s.imageByID[signalID], nil
}

func (s *stubSignalImageRepo) ListRetryCandidates(ctx context.Context, limit int, maxRetryCount int) ([]domain.Signal, error) {
	return nil, nil
}

func (s *stubSignalImageRepo) DeleteExpiredSignalImages(ctx context.Context) (int64, error) {
	return 0, nil
}

type stubSignalChartRenderer struct {
	err error
}

func (s *stubSignalChartRenderer) RenderSignalChart(candles []*domain.Candle, signal domain.Signal) (*domain.SignalImageData, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.SignalImageData{
		Ref: domain.SignalImageRef{
			MimeType: "image/png",
			Width:    640,
			Height:   480,
		},
		Bytes: []byte{0x89, 0x50, 0x4e, 0x47},
	}, nil
}
