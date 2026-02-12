package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	ossignal "os/signal"
	"strings"
	"syscall"
	"time"

	"bug-free-umbrella/internal/cache"
	"bug-free-umbrella/internal/chart"
	"bug-free-umbrella/internal/config"
	"bug-free-umbrella/internal/db"
	"bug-free-umbrella/internal/job"
	mcpserver "bug-free-umbrella/internal/mcp"
	"bug-free-umbrella/internal/provider"
	"bug-free-umbrella/internal/repository"
	"bug-free-umbrella/internal/service"
	signalengine "bug-free-umbrella/internal/signal"
	"bug-free-umbrella/pkg/tracing"

	"github.com/joho/godotenv"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/trace"
)

const defaultMCPHTTPMaxBodyBytes int64 = 1 << 20 // 1MiB

var (
	loadEnvFunc              = godotenv.Load
	loadConfigFunc           = config.Load
	initPostgresFunc         = db.InitPostgres
	initRedisFunc            = cache.InitRedis
	initTracerFunc           = tracing.InitTracer
	newCandleRepoFunc        = repository.NewCandleRepository
	newSignalRepoFunc        = repository.NewSignalRepository
	newSignalImageRepoFunc   = repository.NewSignalImageRepository
	newMCPServerFunc         = mcpserver.NewServer
	newMCPHandlerFunc        = mcpserver.NewHTTPTransportHandler
	newPriceServiceFunc      = service.NewPriceService
	newSignalServiceFunc     = service.NewSignalServiceWithImages
	newSignalEngineFunc      = signalengine.NewEngine
	newChartRendererFunc     = chart.NewRenderer
	newSignalImageJobFunc    = job.NewSignalImageMaintenance
	startSignalImageJobFunc  = func(j *job.SignalImageMaintenance, ctx context.Context) { go j.Start(ctx) }
	newCoinGeckoProviderFunc = func(tracer trace.Tracer) service.PriceProvider {
		return provider.NewCoinGeckoProvider(tracer)
	}
	runStdioFunc = func(ctx context.Context, server *sdkmcp.Server) error {
		return server.Run(ctx, &sdkmcp.StdioTransport{})
	}
	startHTTPServerFunc  = func(srv *http.Server) error { return srv.ListenAndServe() }
	shutdownHTTPServerFn = func(srv *http.Server, ctx context.Context) error { return srv.Shutdown(ctx) }
	setupSignalNotify    = ossignal.Notify
	waitForSignalFunc    = func(quit <-chan os.Signal) { <-quit }
)

func main() {
	loadEnvFunc()
	cfg := loadConfigFunc()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	os.Setenv("DATABASE_URL", cfg.DatabaseURL)
	os.Setenv("REDIS_URL", cfg.RedisURL)
	initPostgresFunc(ctx)
	initRedisFunc(ctx)

	tp, tracer, err := initTracerFunc(ctx)
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down tracer provider: %v", err)
		}
	}()

	candleRepo := newCandleRepoFunc(db.Pool, tracer)
	signalRepo := newSignalRepoFunc(db.Pool, tracer)
	signalImageRepo := newSignalImageRepoFunc(db.Pool, tracer)
	cgProvider := newCoinGeckoProviderFunc(tracer)
	priceService := newPriceServiceFunc(tracer, cgProvider, candleRepo, cache.Client)
	signalEngine := newSignalEngineFunc(nil)
	chartRenderer := newChartRendererFunc()
	signalService := newSignalServiceFunc(tracer, candleRepo, signalRepo, signalEngine, signalImageRepo, chartRenderer)
	imageJob := newSignalImageJobFunc(tracer, signalService)
	startSignalImageJobFunc(imageJob, ctx)

	mcpSrv := newMCPServerFunc(tracer, priceService, signalService, mcpserver.ServerConfig{
		RequestTimeout: time.Duration(cfg.MCPRequestTimeoutSecs) * time.Second,
	})

	transport := strings.ToLower(strings.TrimSpace(cfg.MCPTransport))
	switch transport {
	case "", "stdio":
		if err := runStdioFunc(ctx, mcpSrv); err != nil {
			log.Fatalf("mcp stdio server failed: %v", err)
		}
	case "http":
		if err := runHTTPMode(ctx, cancel, cfg, mcpSrv); err != nil {
			log.Fatalf("mcp http server failed: %v", err)
		}
	default:
		log.Fatalf("unsupported MCP_TRANSPORT: %s", cfg.MCPTransport)
	}
}

func runHTTPMode(ctx context.Context, cancel context.CancelFunc, cfg *config.Config, mcpSrv *sdkmcp.Server) error {
	if !cfg.MCPHTTPEnabled {
		return fmt.Errorf("MCP_HTTP_ENABLED must be true when MCP_TRANSPORT=http")
	}
	if strings.TrimSpace(cfg.MCPAuthToken) == "" {
		return fmt.Errorf("MCP_AUTH_TOKEN is required when MCP_TRANSPORT=http")
	}

	handler := newMCPHandlerFunc(mcpSrv, mcpserver.HTTPHandlerConfig{
		AuthToken:       cfg.MCPAuthToken,
		RateLimitPerMin: cfg.MCPRateLimitPerMin,
		MaxBodyBytes:    defaultMCPHTTPMaxBodyBytes,
	})

	addr := net.JoinHostPort(cfg.MCPHTTPBind, fmt.Sprintf("%d", cfg.MCPHTTPPort))
	srv := &http.Server{Addr: addr, Handler: handler}

	go func() {
		if err := startHTTPServerFunc(srv); err != nil && err != http.ErrServerClosed {
			log.Printf("mcp http server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	setupSignalNotify(quit, syscall.SIGINT, syscall.SIGTERM)
	waitForSignalFunc(quit)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := shutdownHTTPServerFn(srv, shutdownCtx); err != nil {
		return fmt.Errorf("mcp server forced to shutdown: %w", err)
	}
	return nil
}
