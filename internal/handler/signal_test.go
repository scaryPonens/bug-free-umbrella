package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
			ID:        1,
			Symbol:    "BTC",
			Interval:  "1h",
			Indicator: domain.IndicatorRSI,
			Direction: domain.DirectionLong,
			Risk:      domain.RiskLevel2,
			Timestamp: time.Unix(0, 0).UTC(),
			Image: &domain.SignalImageRef{
				ImageID:   101,
				MimeType:  "image/png",
				Width:     640,
				Height:    480,
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			},
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
	if resp.Signals[0].Image == nil || resp.Signals[0].Image.ImageID != 101 {
		t.Fatalf("expected signal image metadata in response, got %+v", resp.Signals[0].Image)
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

func TestGetSignalImageSuccess(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	imageRepo := &handlerSignalImageRepoStub{
		imageBySignalID: map[int64]*domain.SignalImageData{
			42: {
				Ref: domain.SignalImageRef{
					ImageID:   7,
					MimeType:  "image/png",
					Width:     10,
					Height:    10,
					ExpiresAt: time.Now().UTC().Add(time.Hour),
				},
				Bytes: []byte{0x89, 0x50, 0x4e, 0x47},
			},
		},
	}
	h := &Handler{
		tracer: tracer,
		signalService: service.NewSignalServiceWithImages(
			tracer,
			&stubRepo{},
			&handlerSignalStoreStub{},
			stubSignalEngine{},
			imageRepo,
			nil,
		),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/signals/42/image", nil)
	router := gin.New()
	router.GET("/api/signals/:id/image", h.GetSignalImage)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "image/png") {
		t.Fatalf("expected image/png content-type, got %s", got)
	}
	if len(w.Body.Bytes()) == 0 {
		t.Fatal("expected non-empty image bytes")
	}
}

func TestGetSignalImageNotFound(t *testing.T) {
	tracer := trace.NewNoopTracerProvider().Tracer("handler-test")
	h := &Handler{
		tracer: tracer,
		signalService: service.NewSignalServiceWithImages(
			tracer,
			&stubRepo{},
			&handlerSignalStoreStub{},
			stubSignalEngine{},
			&handlerSignalImageRepoStub{},
			nil,
		),
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/signals/999/image", nil)
	router := gin.New()
	router.GET("/api/signals/:id/image", h.GetSignalImage)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

type handlerSignalStoreStub struct {
	lastFilter domain.SignalFilter
	resp       []domain.Signal
}

func (s *handlerSignalStoreStub) InsertSignals(ctx context.Context, signals []domain.Signal) ([]domain.Signal, error) {
	return append([]domain.Signal(nil), signals...), nil
}

func (s *handlerSignalStoreStub) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.lastFilter = filter
	return append([]domain.Signal(nil), s.resp...), nil
}

type handlerSignalImageRepoStub struct {
	imageBySignalID map[int64]*domain.SignalImageData
}

func (s *handlerSignalImageRepoStub) UpsertSignalImageReady(
	ctx context.Context,
	signalID int64,
	imageBytes []byte,
	mimeType string,
	width, height int,
	expiresAt time.Time,
) (*domain.SignalImageRef, error) {
	return nil, nil
}

func (s *handlerSignalImageRepoStub) UpsertSignalImageFailure(
	ctx context.Context,
	signalID int64,
	errorText string,
	nextRetryAt time.Time,
	expiresAt time.Time,
) error {
	return nil
}

func (s *handlerSignalImageRepoStub) GetSignalImageBySignalID(ctx context.Context, signalID int64) (*domain.SignalImageData, error) {
	if s.imageBySignalID == nil {
		return nil, nil
	}
	if img, ok := s.imageBySignalID[signalID]; ok {
		copy := *img
		copy.Bytes = append([]byte(nil), img.Bytes...)
		return &copy, nil
	}
	return nil, nil
}

func (s *handlerSignalImageRepoStub) ListRetryCandidates(ctx context.Context, limit int, maxRetryCount int) ([]domain.Signal, error) {
	return nil, nil
}

func (s *handlerSignalImageRepoStub) DeleteExpiredSignalImages(ctx context.Context) (int64, error) {
	return 0, nil
}
