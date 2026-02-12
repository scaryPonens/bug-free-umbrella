package job

import (
	"context"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

func TestSignalPollerStart(t *testing.T) {
	t.Parallel()

	tracer := trace.NewNoopTracerProvider().Tracer("test")
	stub := &stubSignalService{}
	alerts := &stubSignalAlerter{}
	poller := NewSignalPoller(tracer, stub, alerts)

	ctx, cancel := context.WithCancel(context.Background())
	go poller.Start(ctx)

	eventuallySignal(t, func() bool { return stub.calls > 0 })
	cancel()
}

func TestSignalPollerFetchShortBatch(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	stub := &stubSignalService{
		toReturn: []domain.Signal{{
			Symbol:    "BTC",
			Interval:  "5m",
			Indicator: domain.IndicatorRSI,
			Direction: domain.DirectionLong,
			Risk:      domain.RiskLevel3,
			Timestamp: time.Unix(0, 0).UTC(),
		}},
	}
	alerts := &stubSignalAlerter{}
	poller := NewSignalPoller(tracer, stub, alerts)

	idx := 0
	poller.fetchShortBatch(context.Background(), &idx, 3)

	if len(stub.symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(stub.symbols))
	}
	if stub.symbols[0] != domain.SupportedSymbols[0] {
		t.Fatalf("unexpected symbol order: %+v", stub.symbols)
	}
	if len(stub.intervals) == 0 || len(stub.intervals[0]) != 3 {
		t.Fatalf("unexpected interval set: %+v", stub.intervals)
	}
	if alerts.notifyCalls != 1 {
		t.Fatalf("expected one alert dispatch, got %d", alerts.notifyCalls)
	}
}

func TestSignalPollerFetchLongBatch(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	stub := &stubSignalService{}
	poller := NewSignalPoller(tracer, stub, nil)

	idx := 0
	poller.fetchLongBatch(context.Background(), &idx)

	if len(stub.symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(stub.symbols))
	}
	if len(stub.intervals[0]) != 2 {
		t.Fatalf("expected 2 long intervals, got %d", len(stub.intervals[0]))
	}
}

func TestSignalPollerDedupeAlerts(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	alerts := &stubSignalAlerter{}
	poller := NewSignalPoller(tracer, &stubSignalService{}, alerts)

	sig := domain.Signal{
		Symbol:    "BTC",
		Interval:  "1h",
		Indicator: domain.IndicatorMACD,
		Direction: domain.DirectionLong,
		Risk:      domain.RiskLevel4,
		Timestamp: time.Unix(100, 0).UTC(),
	}

	poller.notifySignals(context.Background(), []domain.Signal{sig})
	poller.notifySignals(context.Background(), []domain.Signal{sig})

	if alerts.notifyCalls != 1 {
		t.Fatalf("expected deduped single dispatch, got %d", alerts.notifyCalls)
	}
}

type stubSignalService struct {
	calls     int
	symbols   []string
	intervals [][]string
	toReturn  []domain.Signal
}

func (s *stubSignalService) GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error) {
	s.calls++
	s.symbols = append(s.symbols, symbol)
	s.intervals = append(s.intervals, append([]string(nil), intervals...))
	return append([]domain.Signal(nil), s.toReturn...), nil
}

type stubSignalAlerter struct {
	notifyCalls int
	lastSignals []domain.Signal
}

func (s *stubSignalAlerter) NotifySignals(ctx context.Context, signals []domain.Signal) error {
	s.notifyCalls++
	s.lastSignals = append([]domain.Signal(nil), signals...)
	return nil
}

func eventuallySignal(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met")
}
