package job

import (
	"context"
	"log"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

var (
	shortSignalIntervals = []string{"5m", "15m", "1h"}
	longSignalIntervals  = []string{"4h", "1d"}
)

// SignalPoller periodically computes and stores technical signals.
type SignalPoller struct {
	tracer        trace.Tracer
	signalService SignalGenerator
}

type SignalGenerator interface {
	GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error)
}

func NewSignalPoller(tracer trace.Tracer, signalService SignalGenerator) *SignalPoller {
	return &SignalPoller{
		tracer:        tracer,
		signalService: signalService,
	}
}

// Start launches background signal generation goroutines. Blocks until ctx is cancelled.
func (p *SignalPoller) Start(ctx context.Context) {
	if p.signalService == nil {
		log.Println("Signal poller disabled: no signal service")
		<-ctx.Done()
		return
	}

	log.Println("Signal poller starting...")
	go p.pollShortSignals(ctx)
	go p.pollLongSignals(ctx)

	<-ctx.Done()
	log.Println("Signal poller stopped")
}

func (p *SignalPoller) pollShortSignals(ctx context.Context) {
	coinIndex := 0
	coinsPerTick := 2

	p.fetchShortBatch(ctx, &coinIndex, coinsPerTick)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchShortBatch(ctx, &coinIndex, coinsPerTick)
		}
	}
}

func (p *SignalPoller) fetchShortBatch(ctx context.Context, coinIndex *int, count int) {
	symbols := domain.SupportedSymbols
	for i := 0; i < count; i++ {
		symbol := symbols[*coinIndex%len(symbols)]
		*coinIndex++

		if _, err := p.signalService.GenerateForSymbol(ctx, symbol, shortSignalIntervals); err != nil {
			log.Printf("short signal generation error for %s: %v", symbol, err)
		}
	}
}

func (p *SignalPoller) pollLongSignals(ctx context.Context) {
	coinIndex := 0

	p.fetchLongBatch(ctx, &coinIndex)

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.fetchLongBatch(ctx, &coinIndex)
		}
	}
}

func (p *SignalPoller) fetchLongBatch(ctx context.Context, coinIndex *int) {
	symbols := domain.SupportedSymbols
	symbol := symbols[*coinIndex%len(symbols)]
	*coinIndex++

	if _, err := p.signalService.GenerateForSymbol(ctx, symbol, longSignalIntervals); err != nil {
		log.Printf("long signal generation error for %s: %v", symbol, err)
	}
}
