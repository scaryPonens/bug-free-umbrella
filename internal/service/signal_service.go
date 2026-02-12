package service

import (
	"context"
	"fmt"
	"strings"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

const signalLookbackCandles = 250

type SignalCandleRepository interface {
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*domain.Candle, error)
}

type SignalRepository interface {
	InsertSignals(ctx context.Context, signals []domain.Signal) error
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

type SignalEngine interface {
	Generate(candles []*domain.Candle) []domain.Signal
}

type SignalService struct {
	tracer     trace.Tracer
	candleRepo SignalCandleRepository
	signalRepo SignalRepository
	engine     SignalEngine
}

func NewSignalService(
	tracer trace.Tracer,
	candleRepo SignalCandleRepository,
	signalRepo SignalRepository,
	engine SignalEngine,
) *SignalService {
	return &SignalService{
		tracer:     tracer,
		candleRepo: candleRepo,
		signalRepo: signalRepo,
		engine:     engine,
	}
}

func (s *SignalService) GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error) {
	_, span := s.tracer.Start(ctx, "signal-service.generate-for-symbol")
	defer span.End()

	if s.candleRepo == nil || s.signalRepo == nil || s.engine == nil {
		return nil, fmt.Errorf("signal service is not fully initialized")
	}

	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if _, ok := domain.CoinGeckoID[symbol]; !ok {
		return nil, fmt.Errorf("unsupported symbol: %s", symbol)
	}

	if len(intervals) == 0 {
		intervals = domain.SupportedIntervals
	}

	generated := make([]domain.Signal, 0, len(intervals)*2)
	for _, interval := range intervals {
		candles, err := s.candleRepo.GetCandles(ctx, symbol, interval, signalLookbackCandles)
		if err != nil {
			return nil, fmt.Errorf("get candles for %s %s: %w", symbol, interval, err)
		}
		if len(candles) == 0 {
			continue
		}

		intervalSignals := s.engine.Generate(candles)
		generated = append(generated, intervalSignals...)
	}

	if len(generated) > 0 {
		if err := s.signalRepo.InsertSignals(ctx, generated); err != nil {
			return nil, fmt.Errorf("insert signals: %w", err)
		}
	}

	return generated, nil
}

func (s *SignalService) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	_, span := s.tracer.Start(ctx, "signal-service.list-signals")
	defer span.End()

	if s.signalRepo == nil {
		return nil, fmt.Errorf("signal service is not fully initialized")
	}

	filter.Symbol = strings.ToUpper(strings.TrimSpace(filter.Symbol))
	filter.Indicator = strings.ToLower(strings.TrimSpace(filter.Indicator))

	if filter.Symbol != "" {
		if _, ok := domain.CoinGeckoID[filter.Symbol]; !ok {
			return nil, fmt.Errorf("unsupported symbol: %s", filter.Symbol)
		}
	}
	if filter.Risk != nil && !filter.Risk.IsValid() {
		return nil, fmt.Errorf("invalid risk level: %d", *filter.Risk)
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	return s.signalRepo.ListSignals(ctx, filter)
}
