package webconsole

import (
	"context"
	"fmt"
	"strings"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"
)

type AdvisorReader interface {
	Ask(ctx context.Context, chatID int64, message string) (string, error)
}

type PriceReader interface {
	GetCurrentPrices(ctx context.Context) ([]*domain.PriceSnapshot, error)
}

type SignalReader interface {
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

type BacktestReader interface {
	GetSummary(ctx context.Context) ([]repository.DailyAccuracy, error)
	GetDaily(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error)
	GetPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error)
}

type DashboardSnapshot struct {
	Prices  []*domain.PriceSnapshot `json:"prices"`
	Signals []domain.Signal         `json:"signals"`
}

type BacktestSnapshot struct {
	Summary     []repository.DailyAccuracy `json:"summary"`
	Daily       []repository.DailyAccuracy `json:"daily"`
	Predictions []domain.MLPrediction      `json:"predictions"`
}

type Service struct {
	prices   PriceReader
	signals  SignalReader
	backtest BacktestReader
	advisor  AdvisorReader
}

func NewService(prices PriceReader, signals SignalReader, backtest BacktestReader, advisor AdvisorReader) *Service {
	return &Service{
		prices:   prices,
		signals:  signals,
		backtest: backtest,
		advisor:  advisor,
	}
}

func (s *Service) Ask(ctx context.Context, sessionID, message string) (string, error) {
	if s.advisor == nil {
		return "", fmt.Errorf("advisor unavailable")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	chatID := chatIDFromSession(sessionID)
	return s.advisor.Ask(ctx, chatID, message)
}

func (s *Service) DefaultDashboardSignalLimit() int { return 10 }

func (s *Service) DefaultSignalsLimit() int { return 100 }

func (s *Service) DefaultBacktestDays() int { return 30 }

func (s *Service) DefaultBacktestPredictionsLimit() int { return 50 }

func (s *Service) GetDashboard(ctx context.Context) (DashboardSnapshot, error) {
	if s.prices == nil || s.signals == nil {
		return DashboardSnapshot{}, fmt.Errorf("dashboard services unavailable")
	}
	prices, err := s.prices.GetCurrentPrices(ctx)
	if err != nil {
		return DashboardSnapshot{}, err
	}
	signals, err := s.signals.ListSignals(ctx, domain.SignalFilter{Limit: s.DefaultDashboardSignalLimit()})
	if err != nil {
		return DashboardSnapshot{}, err
	}
	return DashboardSnapshot{Prices: prices, Signals: signals}, nil
}

func (s *Service) GetSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	if s.signals == nil {
		return nil, fmt.Errorf("signal service unavailable")
	}
	if filter.Limit <= 0 {
		filter.Limit = s.DefaultSignalsLimit()
	}
	return s.signals.ListSignals(ctx, filter)
}

func (s *Service) GetBacktest(ctx context.Context, modelKey string) (BacktestSnapshot, error) {
	if s.backtest == nil {
		return BacktestSnapshot{}, fmt.Errorf("backtest service unavailable")
	}
	summary, err := s.backtest.GetSummary(ctx)
	if err != nil {
		return BacktestSnapshot{}, err
	}
	daily, err := s.backtest.GetDaily(ctx, modelKey, s.DefaultBacktestDays())
	if err != nil {
		return BacktestSnapshot{}, err
	}
	predictions, err := s.backtest.GetPredictions(ctx, s.DefaultBacktestPredictionsLimit())
	if err != nil {
		return BacktestSnapshot{}, err
	}
	return BacktestSnapshot{
		Summary:     summary,
		Daily:       daily,
		Predictions: predictions,
	}, nil
}

func chatIDFromSession(sessionID string) int64 {
	var out int64
	for i := 0; i < len(sessionID); i++ {
		out += int64(sessionID[i])
	}
	return -(out + 2_000_000)
}
