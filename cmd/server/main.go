package main

import (
	"context"
	"log"
	"net/http"
	"os"
	ossignal "os/signal"
	"syscall"
	"time"

	"bug-free-umbrella/internal/bot"
	"bug-free-umbrella/internal/cache"
	"bug-free-umbrella/internal/config"
	"bug-free-umbrella/internal/db"
	"bug-free-umbrella/internal/handler"
	"bug-free-umbrella/internal/job"
	"bug-free-umbrella/internal/provider"
	"bug-free-umbrella/internal/repository"
	"bug-free-umbrella/internal/service"
	signalengine "bug-free-umbrella/internal/signal"
	"bug-free-umbrella/pkg/tracing"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"

	_ "bug-free-umbrella/docs"
)

var (
	loadEnvFunc              = godotenv.Load
	loadConfigFunc           = config.Load
	initPostgresFunc         = db.InitPostgres
	initRedisFunc            = cache.InitRedis
	initTracerFunc           = tracing.InitTracer
	newCandleRepoFunc        = repository.NewCandleRepository
	newSignalRepoFunc        = repository.NewSignalRepository
	newCoinGeckoProviderFunc = func(tracer trace.Tracer) service.PriceProvider {
		return provider.NewCoinGeckoProvider(tracer)
	}
	newSignalEngineFunc    = signalengine.NewEngine
	newPriceServiceFunc    = service.NewPriceService
	newSignalServiceFunc   = service.NewSignalService
	newPricePollerFunc     = job.NewPricePoller
	newSignalPollerFunc    = job.NewSignalPoller
	startPollerFunc        = func(p *job.PricePoller, ctx context.Context) { go p.Start(ctx) }
	startSignalPollerFunc  = func(p *job.SignalPoller, ctx context.Context) { go p.Start(ctx) }
	startTelegramBotFunc   = bot.StartTelegramBot
	newWorkServiceFunc     = service.NewWorkService
	newHandlerFunc         = handler.New
	newRouterFunc          = gin.Default
	setupSignalNotify      = ossignal.Notify
	waitForSignalFunc      = func(quit <-chan os.Signal) { <-quit }
	startHTTPServerFunc    = func(srv *http.Server) error { return srv.ListenAndServe() }
	shutdownHTTPServerFunc = func(srv *http.Server, ctx context.Context) error { return srv.Shutdown(ctx) }
)

// @title           Bug Free Umbrella API
// @version         1.0
// @description     A Go service with OpenTelemetry tracing.

// @host      localhost:8080
// @BasePath  /
func main() {
	loadEnvFunc()

	cfg := loadConfigFunc()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Init Postgres and Redis
	os.Setenv("DATABASE_URL", cfg.DatabaseURL)
	os.Setenv("REDIS_URL", cfg.RedisURL)
	initPostgresFunc(ctx)
	initRedisFunc(ctx)

	// Init tracing
	tp, tracer, err := initTracerFunc(ctx)
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("error shutting down tracer provider: %v", err)
		}
	}()

	// Create repository and run migrations
	candleRepo := newCandleRepoFunc(db.Pool, tracer)
	signalRepo := newSignalRepoFunc(db.Pool, tracer)
	if db.Pool != nil {
		if err := candleRepo.RunMigrations(ctx); err != nil {
			log.Fatalf("failed to run migrations: %v", err)
		}
		if err := signalRepo.RunMigrations(ctx); err != nil {
			log.Fatalf("failed to run signal migrations: %v", err)
		}
	}

	// Create providers and services
	cgProvider := newCoinGeckoProviderFunc(tracer)
	priceService := newPriceServiceFunc(tracer, cgProvider, candleRepo, cache.Client)
	signalEngine := newSignalEngineFunc(nil)
	signalService := newSignalServiceFunc(tracer, candleRepo, signalRepo, signalEngine)

	// Start background pollers (stopped by ctx cancel)
	poller := newPricePollerFunc(tracer, priceService, cfg.CoinGeckoPollSecs)
	startPollerFunc(poller, ctx)
	signalPoller := newSignalPollerFunc(tracer, signalService)
	startSignalPollerFunc(signalPoller, ctx)

	// Start Telegram bot
	os.Setenv("TELEGRAM_BOT_TOKEN", cfg.TelegramBotToken)
	startTelegramBotFunc(priceService, signalService)

	// Create handlers and routes
	workService := newWorkServiceFunc(tracer)
	h := newHandlerFunc(tracer, workService, priceService, signalService)

	r := newRouterFunc()
	r.Use(otelgin.Middleware("bug-free-umbrella"))

	h.RegisterRoutes(r)
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		if err := startHTTPServerFunc(srv); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	setupSignalNotify(quit, syscall.SIGINT, syscall.SIGTERM)
	waitForSignalFunc(quit)
	log.Println("Shutting down server...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := shutdownHTTPServerFunc(srv, shutdownCtx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
