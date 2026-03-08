package service

import (
	"context"
	"errors"
	"testing"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"

	"go.opentelemetry.io/otel/trace"
)

type backtestRepoStub struct {
	summaryErr error
	dailyErr   error
	predErr    error
}

func (s backtestRepoStub) GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error) {
	if s.dailyErr != nil {
		return nil, s.dailyErr
	}
	return []repository.DailyAccuracy{{ModelKey: "ml", Total: 10, Correct: 7, Accuracy: 0.7}}, nil
}

func (s backtestRepoStub) GetAccuracySummary(ctx context.Context) ([]repository.DailyAccuracy, error) {
	if s.summaryErr != nil {
		return nil, s.summaryErr
	}
	return []repository.DailyAccuracy{{ModelKey: "ml", Total: 10, Correct: 7, Accuracy: 0.7}}, nil
}

func (s backtestRepoStub) ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	if s.predErr != nil {
		return nil, s.predErr
	}
	return []domain.MLPrediction{{ModelKey: "ml", Symbol: "BTC"}}, nil
}

func TestBacktestServiceGetSummary(t *testing.T) {
	svc := NewBacktestService(trace.NewNoopTracerProvider().Tracer("test"), backtestRepoStub{})
	items, err := svc.GetSummary(context.Background())
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
}

func TestBacktestServiceGetSummaryError(t *testing.T) {
	svc := NewBacktestService(trace.NewNoopTracerProvider().Tracer("test"), backtestRepoStub{summaryErr: errors.New("boom")})
	if _, err := svc.GetSummary(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
