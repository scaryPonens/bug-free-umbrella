package job

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"bug-free-umbrella/internal/domain"

	"go.opentelemetry.io/otel/trace"
)

var (
	shortSignalIntervals = []string{"5m", "15m", "1h"}
	longSignalIntervals  = []string{"4h", "1d"}
)

const maxSeenAlertSignals = 10000

// SignalPoller periodically computes and stores technical signals.
type SignalPoller struct {
	tracer        trace.Tracer
	signalService SignalGenerator
	alertSink     SignalAlertSink

	alertMu        sync.Mutex
	seenAlertKeys  map[string]struct{}
	seenAlertOrder []string
}

type SignalGenerator interface {
	GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error)
}

type SignalAlertSink interface {
	NotifySignals(ctx context.Context, signals []domain.Signal) error
}

func NewSignalPoller(tracer trace.Tracer, signalService SignalGenerator, alertSink SignalAlertSink) *SignalPoller {
	return &SignalPoller{
		tracer:        tracer,
		signalService: signalService,
		alertSink:     alertSink,
		seenAlertKeys: make(map[string]struct{}),
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

		signals, err := p.signalService.GenerateForSymbol(ctx, symbol, shortSignalIntervals)
		if err != nil {
			log.Printf("short signal generation error for %s: %v", symbol, err)
			continue
		}
		p.notifySignals(ctx, signals)
	}
}

func (p *SignalPoller) notifySignals(ctx context.Context, generated []domain.Signal) {
	if p.alertSink == nil || len(generated) == 0 {
		return
	}

	fresh := p.filterUnseenSignals(generated)
	if len(fresh) == 0 {
		return
	}
	if err := p.alertSink.NotifySignals(ctx, fresh); err != nil {
		log.Printf("signal alert dispatch error: %v", err)
	}
}

func (p *SignalPoller) filterUnseenSignals(generated []domain.Signal) []domain.Signal {
	p.alertMu.Lock()
	defer p.alertMu.Unlock()

	fresh := make([]domain.Signal, 0, len(generated))
	for _, s := range generated {
		key := signalAlertKey(s)
		if _, exists := p.seenAlertKeys[key]; exists {
			continue
		}
		p.seenAlertKeys[key] = struct{}{}
		p.seenAlertOrder = append(p.seenAlertOrder, key)
		fresh = append(fresh, s)
	}

	if overflow := len(p.seenAlertOrder) - maxSeenAlertSignals; overflow > 0 {
		for i := 0; i < overflow; i++ {
			oldest := p.seenAlertOrder[0]
			p.seenAlertOrder = p.seenAlertOrder[1:]
			delete(p.seenAlertKeys, oldest)
		}
	}
	return fresh
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

	signals, err := p.signalService.GenerateForSymbol(ctx, symbol, longSignalIntervals)
	if err != nil {
		log.Printf("long signal generation error for %s: %v", symbol, err)
		return
	}
	p.notifySignals(ctx, signals)
}

func signalAlertKey(s domain.Signal) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%d",
		s.Symbol,
		s.Interval,
		s.Indicator,
		s.Direction,
		s.Timestamp.UTC().Unix(),
	)
}
