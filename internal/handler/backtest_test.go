package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"
	"bug-free-umbrella/internal/service"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

type backtestRepoForHandler struct{}

func (backtestRepoForHandler) GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error) {
	return []repository.DailyAccuracy{{ModelKey: "ml_logreg_up4h", Total: 10, Correct: 7, Accuracy: 0.7}}, nil
}

func (backtestRepoForHandler) GetAccuracySummary(ctx context.Context) ([]repository.DailyAccuracy, error) {
	return []repository.DailyAccuracy{{ModelKey: "ml_logreg_up4h", Total: 20, Correct: 14, Accuracy: 0.7}}, nil
}

func (backtestRepoForHandler) ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	return []domain.MLPrediction{{ModelKey: "ml_logreg_up4h", Symbol: "BTC"}}, nil
}

func TestGetBacktestSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	h := &Handler{tracer: tracer, backtestService: service.NewBacktestService(tracer, backtestRepoForHandler{})}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/backtest/summary", nil)
	r := gin.New()
	r.GET("/api/backtest/summary", h.GetBacktestSummary)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := payload["summary"]; !ok {
		t.Fatalf("expected summary field")
	}
}
