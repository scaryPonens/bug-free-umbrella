package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

const (
	signalLookbackCandles = 250
	signalImageTTL        = 24 * time.Hour
	signalImageRetryDelay = 5 * time.Minute
	defaultImageRetryMax  = 3
)

type SignalCandleRepository interface {
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*domain.Candle, error)
}

type SignalRepository interface {
	InsertSignals(ctx context.Context, signals []domain.Signal) ([]domain.Signal, error)
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

type SignalEngine interface {
	Generate(candles []*domain.Candle) []domain.Signal
}

type SignalImageRepository interface {
	UpsertSignalImageReady(
		ctx context.Context,
		signalID int64,
		imageBytes []byte,
		mimeType string,
		width, height int,
		expiresAt time.Time,
	) (*domain.SignalImageRef, error)
	UpsertSignalImageFailure(
		ctx context.Context,
		signalID int64,
		errorText string,
		nextRetryAt time.Time,
		expiresAt time.Time,
	) error
	GetSignalImageBySignalID(ctx context.Context, signalID int64) (*domain.SignalImageData, error)
	ListRetryCandidates(ctx context.Context, limit int, maxRetryCount int) ([]domain.Signal, error)
	DeleteExpiredSignalImages(ctx context.Context) (int64, error)
}

type SignalChartRenderer interface {
	RenderSignalChart(candles []*domain.Candle, signal domain.Signal) (*domain.SignalImageData, error)
}

type SignalService struct {
	tracer        trace.Tracer
	candleRepo    SignalCandleRepository
	signalRepo    SignalRepository
	engine        SignalEngine
	imageRepo     SignalImageRepository
	chartRender   SignalChartRenderer
	maxImageRetry int
}

func NewSignalService(
	tracer trace.Tracer,
	candleRepo SignalCandleRepository,
	signalRepo SignalRepository,
	engine SignalEngine,
) *SignalService {
	return NewSignalServiceWithImages(tracer, candleRepo, signalRepo, engine, nil, nil)
}

func NewSignalServiceWithImages(
	tracer trace.Tracer,
	candleRepo SignalCandleRepository,
	signalRepo SignalRepository,
	engine SignalEngine,
	imageRepo SignalImageRepository,
	chartRender SignalChartRenderer,
) *SignalService {
	return &SignalService{
		tracer:        tracer,
		candleRepo:    candleRepo,
		signalRepo:    signalRepo,
		engine:        engine,
		imageRepo:     imageRepo,
		chartRender:   chartRender,
		maxImageRetry: defaultImageRetryMax,
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
	candlesByInterval := make(map[string][]*domain.Candle, len(intervals))
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
		candlesByInterval[interval] = candles
	}

	if len(generated) > 0 {
		persisted, err := s.signalRepo.InsertSignals(ctx, generated)
		if err != nil {
			return nil, fmt.Errorf("insert signals: %w", err)
		}
		generated = persisted
		s.attachGeneratedSignalImages(ctx, generated, candlesByInterval)
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

func (s *SignalService) GetSignalImage(ctx context.Context, signalID int64) (*domain.SignalImageData, error) {
	_, span := s.tracer.Start(ctx, "signal-service.get-signal-image")
	defer span.End()

	if signalID <= 0 {
		return nil, fmt.Errorf("invalid signal id")
	}
	if s.imageRepo == nil {
		return nil, nil
	}
	return s.imageRepo.GetSignalImageBySignalID(ctx, signalID)
}

func (s *SignalService) RetryFailedImages(ctx context.Context, limit int) (int, error) {
	_, span := s.tracer.Start(ctx, "signal-service.retry-failed-images")
	defer span.End()

	if s.imageRepo == nil || s.chartRender == nil || s.candleRepo == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 20
	}

	candidates, err := s.imageRepo.ListRetryCandidates(ctx, limit, s.maxImageRetry)
	if err != nil {
		return 0, err
	}

	successes := 0
	for _, sig := range candidates {
		candles, err := s.candleRepo.GetCandles(ctx, sig.Symbol, sig.Interval, signalLookbackCandles)
		if err != nil {
			s.recordImageFailure(ctx, sig, fmt.Errorf("get candles for retry: %w", err))
			continue
		}
		if len(candles) == 0 {
			s.recordImageFailure(ctx, sig, fmt.Errorf("no candles available for retry"))
			continue
		}
		if _, err := s.renderAndStoreImage(ctx, sig, candles); err != nil {
			continue
		}
		successes++
	}
	return successes, nil
}

func (s *SignalService) DeleteExpiredSignalImages(ctx context.Context) (int64, error) {
	_, span := s.tracer.Start(ctx, "signal-service.delete-expired-signal-images")
	defer span.End()

	if s.imageRepo == nil {
		return 0, nil
	}
	return s.imageRepo.DeleteExpiredSignalImages(ctx)
}

func (s *SignalService) attachGeneratedSignalImages(
	ctx context.Context,
	generated []domain.Signal,
	candlesByInterval map[string][]*domain.Candle,
) {
	if s.imageRepo == nil || s.chartRender == nil {
		return
	}
	for i := range generated {
		candles := candlesByInterval[generated[i].Interval]
		if len(candles) == 0 {
			continue
		}
		ref, err := s.renderAndStoreImage(ctx, generated[i], candles)
		if err != nil {
			continue
		}
		generated[i].Image = ref
	}
}

func (s *SignalService) renderAndStoreImage(
	ctx context.Context,
	sig domain.Signal,
	candles []*domain.Candle,
) (*domain.SignalImageRef, error) {
	rendered, err := s.chartRender.RenderSignalChart(candles, sig)
	if err != nil {
		s.recordImageFailure(ctx, sig, err)
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(signalImageTTL)
	ref, err := s.imageRepo.UpsertSignalImageReady(
		ctx,
		sig.ID,
		rendered.Bytes,
		rendered.Ref.MimeType,
		rendered.Ref.Width,
		rendered.Ref.Height,
		expiresAt,
	)
	if err != nil {
		s.recordImageFailure(ctx, sig, fmt.Errorf("persist image: %w", err))
		return nil, err
	}
	return ref, nil
}

func (s *SignalService) recordImageFailure(ctx context.Context, sig domain.Signal, err error) {
	if s.imageRepo == nil || sig.ID <= 0 {
		return
	}
	expiresAt := time.Now().UTC().Add(signalImageTTL)
	nextRetry := time.Now().UTC().Add(signalImageRetryDelay)
	if upsertErr := s.imageRepo.UpsertSignalImageFailure(ctx, sig.ID, err.Error(), nextRetry, expiresAt); upsertErr != nil {
		log.Printf("signal image failure upsert error for signal %d: %v", sig.ID, upsertErr)
	}
}
