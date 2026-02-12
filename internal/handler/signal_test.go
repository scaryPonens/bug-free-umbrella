package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/service"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

func TestGetSignalsSuccess(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	repo := &handlerSignalStoreStub{
		resp: []domain.Signal{{
			Symbol:    "BTC",
			Interval:  "1h",
			Indicator: domain.IndicatorRSI,
			Direction: domain.DirectionLong,
			Risk:      domain.RiskLevel2,
			Timestamp: time.Unix(0, 0).UTC(),
		}},
	}
	h := &Handler{
		tracer:        tracer,
		workService:   service.NewWorkService(tracer),
		priceService:  service.NewPriceService(tracer, &stubPriceProvider{}, &stubRepo{}, nil),
		signalService: service.NewSignalService(tracer, &stubRepo{}, repo, stubSignalEngine{}),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/signals?symbol=btc&risk=2&limit=5", nil)

	router := gin.New()
	router.GET("/api/signals", h.GetSignals)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if repo.lastFilter.Symbol != "BTC" {
		t.Fatalf("expected symbol BTC, got %s", repo.lastFilter.Symbol)
	}
	if repo.lastFilter.Limit != 5 {
		t.Fatalf("expected limit 5, got %d", repo.lastFilter.Limit)
	}

	var resp struct {
		Signals []domain.Signal `json:"signals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(resp.Signals) != 1 || resp.Signals[0].Symbol != "BTC" {
		t.Fatalf("unexpected response payload: %+v", resp)
	}
}

func TestGetSignalsInvalidRisk(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	h := &Handler{
		tracer:        tracer,
		signalService: service.NewSignalService(tracer, &stubRepo{}, &handlerSignalStoreStub{}, stubSignalEngine{}),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/signals?risk=9", nil)

	router := gin.New()
	router.GET("/api/signals", h.GetSignals)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetSignalsBadParams(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	h := &Handler{
		tracer:        tracer,
		signalService: service.NewSignalService(tracer, &stubRepo{}, &handlerSignalStoreStub{}, stubSignalEngine{}),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/signals?risk=abc", nil)

	router := gin.New()
	router.GET("/api/signals", h.GetSignals)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

type handlerSignalStoreStub struct {
	lastFilter domain.SignalFilter
	resp       []domain.Signal
}

func (s *handlerSignalStoreStub) InsertSignals(ctx context.Context, signals []domain.Signal) error {
	return nil
}

func (s *handlerSignalStoreStub) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.lastFilter = filter
	return append([]domain.Signal(nil), s.resp...), nil
}
