package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"bug-free-umbrella/internal/domain"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubPriceService struct {
	prices     []*domain.PriceSnapshot
	priceBySym map[string]*domain.PriceSnapshot
	candles    map[string][]*domain.Candle
}

func (s *stubPriceService) GetCurrentPrices(ctx context.Context) ([]*domain.PriceSnapshot, error) {
	return append([]*domain.PriceSnapshot(nil), s.prices...), nil
}

func (s *stubPriceService) GetCurrentPrice(ctx context.Context, symbol string) (*domain.PriceSnapshot, error) {
	if snap, ok := s.priceBySym[symbol]; ok {
		copy := *snap
		return &copy, nil
	}
	return nil, nil
}

func (s *stubPriceService) GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*domain.Candle, error) {
	key := symbol + ":" + interval
	candles := s.candles[key]
	if len(candles) > limit {
		candles = candles[:limit]
	}
	return append([]*domain.Candle(nil), candles...), nil
}

type stubSignalService struct {
	listed    []domain.Signal
	generated []domain.Signal

	lastGenerateSymbol    string
	lastGenerateIntervals []string
	lastFilter            domain.SignalFilter
}

func (s *stubSignalService) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.lastFilter = filter
	return append([]domain.Signal(nil), s.listed...), nil
}

func (s *stubSignalService) GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error) {
	s.lastGenerateSymbol = symbol
	s.lastGenerateIntervals = append([]string(nil), intervals...)
	return append([]domain.Signal(nil), s.generated...), nil
}

func testServer() (*sdkmcp.Server, *stubPriceService, *stubSignalService) {
	prices := &stubPriceService{
		prices: []*domain.PriceSnapshot{
			{Symbol: "BTC", PriceUSD: 50000, Volume24h: 1000, Change24hPct: 2.1, LastUpdatedUnix: time.Now().Unix()},
		},
		priceBySym: map[string]*domain.PriceSnapshot{
			"BTC": {Symbol: "BTC", PriceUSD: 50000, Volume24h: 1000, Change24hPct: 2.1, LastUpdatedUnix: time.Now().Unix()},
		},
		candles: map[string][]*domain.Candle{
			"BTC:1h": {{Symbol: "BTC", Interval: "1h", Open: 1, High: 2, Low: 1, Close: 2, Volume: 3, OpenTime: time.Unix(0, 0).UTC()}},
		},
	}
	signals := &stubSignalService{
		listed: []domain.Signal{{
			ID: 1, Symbol: "BTC", Interval: "1h", Indicator: domain.IndicatorRSI,
			Direction: domain.DirectionLong, Risk: domain.RiskLevel2, Timestamp: time.Unix(0, 0).UTC(),
		}},
		generated: []domain.Signal{{
			ID: 2, Symbol: "BTC", Interval: "1h", Indicator: domain.IndicatorMACD,
			Direction: domain.DirectionLong, Risk: domain.RiskLevel4, Timestamp: time.Unix(1, 0).UTC(),
		}},
	}

	srv := NewServer(nil, prices, signals, ServerConfig{RequestTimeout: time.Second})
	return srv, prices, signals
}

func connectInMemory(ctx context.Context, srv *sdkmcp.Server) (*sdkmcp.ClientSession, context.CancelFunc, error) {
	clientTransport, serverTransport := sdkmcp.NewInMemoryTransports()
	runCtx, cancel := context.WithCancel(ctx)
	go func() { _ = srv.Run(runCtx, serverTransport) }()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "mcp-test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return session, cancel, nil
}

type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if t.token != "" {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

func decodeResourceJSON(result *sdkmcp.ReadResourceResult, out any) error {
	if len(result.Contents) == 0 {
		return nil
	}
	return json.Unmarshal([]byte(result.Contents[0].Text), out)
}
