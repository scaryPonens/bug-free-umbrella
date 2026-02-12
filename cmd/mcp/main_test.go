package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"bug-free-umbrella/internal/config"
	"bug-free-umbrella/internal/domain"
	mcpserver "bug-free-umbrella/internal/mcp"
	"bug-free-umbrella/internal/repository"
	"bug-free-umbrella/internal/service"
	signalengine "bug-free-umbrella/internal/signal"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestMainMCPStdio(t *testing.T) {
	restore := stubMCPDeps(t, "stdio")
	defer restore()

	called := false
	origRunStdio := runStdioFunc
	runStdioFunc = func(ctx context.Context, server *sdkmcp.Server) error {
		called = true
		return nil
	}
	defer func() { runStdioFunc = origRunStdio }()

	main()

	if !called {
		t.Fatal("expected stdio transport to run")
	}
}

func TestMainMCPHTTP(t *testing.T) {
	restore := stubMCPDeps(t, "http")
	defer restore()

	httpStarted := false
	started := make(chan struct{})
	origStartHTTP := startHTTPServerFunc
	origNotify := setupSignalNotify
	origWait := waitForSignalFunc
	origShutdown := shutdownHTTPServerFn

	startHTTPServerFunc = func(*http.Server) error {
		httpStarted = true
		close(started)
		return http.ErrServerClosed
	}
	setupSignalNotify = func(c chan<- os.Signal, sig ...os.Signal) {}
	waitForSignalFunc = func(<-chan os.Signal) { <-started }
	shutdownHTTPServerFn = func(*http.Server, context.Context) error { return nil }

	defer func() {
		startHTTPServerFunc = origStartHTTP
		setupSignalNotify = origNotify
		waitForSignalFunc = origWait
		shutdownHTTPServerFn = origShutdown
	}()

	main()

	if !httpStarted {
		t.Fatal("expected http transport to start")
	}
}

func TestMainMCPHTTPRequiresToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{
		MCPHTTPEnabled: true,
		MCPHTTPBind:    "127.0.0.1",
		MCPHTTPPort:    8090,
	}
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test"}, nil)

	err := runHTTPMode(ctx, cancel, cfg, srv)
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if !strings.Contains(err.Error(), "MCP_AUTH_TOKEN is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubMCPDeps(t *testing.T, transport string) func() {
	t.Helper()

	origLoadEnv := loadEnvFunc
	origLoadConfig := loadConfigFunc
	origInitPostgres := initPostgresFunc
	origInitRedis := initRedisFunc
	origInitTracer := initTracerFunc
	origNewSignalRepo := newSignalRepoFunc
	origNewProvider := newCoinGeckoProviderFunc
	origNewSignalEngine := newSignalEngineFunc
	origNewSignalService := newSignalServiceFunc
	origNewMCPServer := newMCPServerFunc
	origNewMCPHandler := newMCPHandlerFunc

	loadEnvFunc = func(...string) error { return nil }
	loadConfigFunc = func() *config.Config {
		return &config.Config{
			RedisURL:              "",
			DatabaseURL:           "",
			CoinGeckoPollSecs:     1,
			MCPTransport:          transport,
			MCPHTTPEnabled:        true,
			MCPHTTPBind:           "127.0.0.1",
			MCPHTTPPort:           8090,
			MCPAuthToken:          "secret",
			MCPRequestTimeoutSecs: 1,
			MCPRateLimitPerMin:    60,
		}
	}
	initPostgresFunc = func(context.Context) {}
	initRedisFunc = func(context.Context) {}
	initTracerFunc = func(ctx context.Context) (*sdktrace.TracerProvider, trace.Tracer, error) {
		tp := sdktrace.NewTracerProvider()
		return tp, tp.Tracer("test"), nil
	}
	newSignalRepoFunc = func(repository.PgxPool, trace.Tracer) *repository.SignalRepository {
		return nil
	}
	newCoinGeckoProviderFunc = func(trace.Tracer) service.PriceProvider { return stubMCPPriceProvider{} }
	newSignalEngineFunc = func(func() time.Time) *signalengine.Engine { return signalengine.NewEngine(nil) }
	newSignalServiceFunc = func(
		trace.Tracer,
		service.SignalCandleRepository,
		service.SignalRepository,
		service.SignalEngine,
	) *service.SignalService {
		return nil
	}
	newMCPServerFunc = func(trace.Tracer, mcpserver.PriceReader, mcpserver.SignalReaderWriter, mcpserver.ServerConfig) *sdkmcp.Server {
		return sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-mcp"}, nil)
	}
	newMCPHandlerFunc = func(server *sdkmcp.Server, cfg mcpserver.HTTPHandlerConfig) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}

	return func() {
		loadEnvFunc = origLoadEnv
		loadConfigFunc = origLoadConfig
		initPostgresFunc = origInitPostgres
		initRedisFunc = origInitRedis
		initTracerFunc = origInitTracer
		newSignalRepoFunc = origNewSignalRepo
		newCoinGeckoProviderFunc = origNewProvider
		newSignalEngineFunc = origNewSignalEngine
		newSignalServiceFunc = origNewSignalService
		newMCPServerFunc = origNewMCPServer
		newMCPHandlerFunc = origNewMCPHandler
	}
}

type stubMCPPriceProvider struct{}

func (stubMCPPriceProvider) FetchPrices(ctx context.Context) (map[string]*domain.PriceSnapshot, error) {
	return map[string]*domain.PriceSnapshot{"BTC": {Symbol: "BTC", PriceUSD: 1}}, nil
}

func (stubMCPPriceProvider) FetchMarketChart(ctx context.Context, symbol string, days int, intervals []string) ([]*domain.Candle, error) {
	return nil, errors.New("not used")
}
