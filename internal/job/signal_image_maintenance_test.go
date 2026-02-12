package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestSignalImageMaintenanceStartRunsRetryAndCleanup(t *testing.T) {
	stub := &stubSignalImageMaintainer{}
	job := NewSignalImageMaintenance(trace.NewNoopTracerProvider().Tracer("test"), stub)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		job.Start(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("maintenance job did not stop")
	}

	if atomic.LoadInt32(&stub.retryCalls) == 0 {
		t.Fatal("expected retry to run at least once")
	}
	if atomic.LoadInt32(&stub.cleanupCalls) == 0 {
		t.Fatal("expected cleanup to run at least once")
	}
}

type stubSignalImageMaintainer struct {
	retryCalls   int32
	cleanupCalls int32
}

func (s *stubSignalImageMaintainer) RetryFailedImages(ctx context.Context, limit int) (int, error) {
	atomic.AddInt32(&s.retryCalls, 1)
	return 0, nil
}

func (s *stubSignalImageMaintainer) DeleteExpiredSignalImages(ctx context.Context) (int64, error) {
	atomic.AddInt32(&s.cleanupCalls, 1)
	return 0, nil
}
