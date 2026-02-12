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
	poller := NewSignalPoller(tracer, stub)

	ctx, cancel := context.WithCancel(context.Background())
	go poller.Start(ctx)

	eventuallySignal(t, func() bool { return stub.calls > 0 })
	cancel()
}

func TestSignalPollerFetchShortBatch(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	stub := &stubSignalService{}
	poller := NewSignalPoller(tracer, stub)

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
}

func TestSignalPollerFetchLongBatch(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("test")
	stub := &stubSignalService{}
	poller := NewSignalPoller(tracer, stub)

	idx := 0
	poller.fetchLongBatch(context.Background(), &idx)

	if len(stub.symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(stub.symbols))
	}
	if len(stub.intervals[0]) != 2 {
		t.Fatalf("expected 2 long intervals, got %d", len(stub.intervals[0]))
	}
}

type stubSignalService struct {
	calls     int
	symbols   []string
	intervals [][]string
}

func (s *stubSignalService) GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error) {
	s.calls++
	s.symbols = append(s.symbols, symbol)
	s.intervals = append(s.intervals, append([]string(nil), intervals...))
	return nil, nil
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
