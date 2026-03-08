package service

import (
	"context"
	"fmt"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"

	"go.opentelemetry.io/otel/trace"
)

type BacktestRepository interface {
	GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error)
	GetAccuracySummary(ctx context.Context) ([]repository.DailyAccuracy, error)
	ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error)
}

type BacktestService struct {
	tracer trace.Tracer
	repo   BacktestRepository
}

func NewBacktestService(tracer trace.Tracer, repo BacktestRepository) *BacktestService {
	return &BacktestService{tracer: tracer, repo: repo}
}

func (s *BacktestService) GetSummary(ctx context.Context) ([]repository.DailyAccuracy, error) {
	_, span := s.tracer.Start(ctx, "backtest-service.get-summary")
	defer span.End()
	if s.repo == nil {
		return nil, fmt.Errorf("backtest service unavailable")
	}
	return s.repo.GetAccuracySummary(ctx)
}

func (s *BacktestService) GetDaily(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error) {
	_, span := s.tracer.Start(ctx, "backtest-service.get-daily")
	defer span.End()
	if s.repo == nil {
		return nil, fmt.Errorf("backtest service unavailable")
	}
	return s.repo.GetDailyAccuracy(ctx, modelKey, days)
}

func (s *BacktestService) GetPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	_, span := s.tracer.Start(ctx, "backtest-service.get-predictions")
	defer span.End()
	if s.repo == nil {
		return nil, fmt.Errorf("backtest service unavailable")
	}
	return s.repo.ListRecentPredictions(ctx, limit)
}
