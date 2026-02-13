package training

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
	"go.opentelemetry.io/otel/trace"
)

func TestTrainAllIncludesIForestPerInterval(t *testing.T) {
	now := time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC)
	features := &stubFeatureStore{
		labeled: map[string][]domain.MLFeatureRow{
			"1h": makeRows("1h", 420, true),
		},
		rows: map[string][]domain.MLFeatureRow{
			"1h": makeRows("1h", 420, true),
			"4h": makeRows("4h", 420, false),
		},
	}
	registry := newStubRegistry()
	svc := NewService(nilTracer(), features, registry, Config{
		Interval:          "1h",
		Intervals:         []string{"1h", "4h"},
		TrainWindowDays:   90,
		MinTrainSamples:   200,
		EnableIForest:     true,
		IForestTrees:      100,
		IForestSampleSize: 128,
	})

	results, err := svc.TrainAll(context.Background(), now)
	if err != nil {
		t.Fatalf("train all failed: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 model results, got %d", len(results))
	}

	want := map[string]bool{
		"logreg":     false,
		"xgboost":    false,
		"iforest_1h": false,
		"iforest_4h": false,
	}
	for _, r := range results {
		if _, ok := want[r.ModelKey]; ok {
			want[r.ModelKey] = true
		}
		if !r.Promoted {
			t.Fatalf("expected first model version to be promoted for %s", r.ModelKey)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Fatalf("missing result for model key %s", k)
		}
	}
}

func TestShouldPromoteAnomaly(t *testing.T) {
	registry := newStubRegistry()
	key := "iforest_1h"
	registry.active[key] = &domain.MLModelVersion{
		ModelKey:    key,
		Version:     1,
		IsActive:    true,
		MetricsJSON: `{"score_std":0.1200}`,
	}
	svc := NewService(nilTracer(), &stubFeatureStore{}, registry, Config{})

	promote, err := svc.shouldPromoteAnomaly(context.Background(), key, 0.131, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !promote {
		t.Fatal("expected promotion when std improves by >= 0.01")
	}

	promote, err = svc.shouldPromoteAnomaly(context.Background(), key, 0.125, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promote {
		t.Fatal("expected no promotion when std improvement < 0.01")
	}
}

type stubFeatureStore struct {
	labeled map[string][]domain.MLFeatureRow
	rows    map[string][]domain.MLFeatureRow
}

func (s *stubFeatureStore) ListLabeledRows(_ context.Context, interval string, _, _ time.Time) ([]domain.MLFeatureRow, error) {
	return append([]domain.MLFeatureRow(nil), s.labeled[interval]...), nil
}

func (s *stubFeatureStore) ListRows(_ context.Context, interval string, _, _ time.Time) ([]domain.MLFeatureRow, error) {
	return append([]domain.MLFeatureRow(nil), s.rows[interval]...), nil
}

type stubRegistry struct {
	mu     sync.Mutex
	next   map[string]int
	models map[string]*domain.MLModelVersion
	active map[string]*domain.MLModelVersion
}

func newStubRegistry() *stubRegistry {
	return &stubRegistry{
		next:   make(map[string]int),
		models: make(map[string]*domain.MLModelVersion),
		active: make(map[string]*domain.MLModelVersion),
	}
}

func (s *stubRegistry) NextVersion(_ context.Context, modelKey string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next[modelKey]++
	return s.next[modelKey], nil
}

func (s *stubRegistry) InsertModelVersion(_ context.Context, model domain.MLModelVersion) (*domain.MLModelVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := registryModelKey(model.ModelKey, model.Version)
	copyModel := model
	s.models[key] = &copyModel
	return &copyModel, nil
}

func (s *stubRegistry) GetActiveModel(_ context.Context, modelKey string) (*domain.MLModelVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if model, ok := s.active[modelKey]; ok {
		copyModel := *model
		return &copyModel, nil
	}
	return nil, nil
}

func (s *stubRegistry) ActivateModel(_ context.Context, modelKey string, version int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := registryModelKey(modelKey, version)
	model, ok := s.models[key]
	if !ok {
		return fmt.Errorf("model not found for activation: %s", key)
	}
	copyModel := *model
	copyModel.IsActive = true
	s.active[modelKey] = &copyModel
	return nil
}

func registryModelKey(modelKey string, version int) string {
	return fmt.Sprintf("%s:%d", modelKey, version)
}

func makeRows(interval string, n int, labeled bool) []domain.MLFeatureRow {
	rows := make([]domain.MLFeatureRow, 0, n)
	start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		up := (i%3 != 0)
		var target *bool
		if labeled {
			target = &up
		}
		base := float64(i) / float64(n)
		if !up {
			base = -base
		}
		rows = append(rows, domain.MLFeatureRow{
			Symbol:        "BTC",
			Interval:      interval,
			OpenTime:      start.Add(time.Duration(i) * time.Hour),
			Ret1H:         base,
			Ret4H:         base * 0.8,
			Ret12H:        base * 0.6,
			Ret24H:        base * 0.4,
			Volatility6H:  0.01 + (float64(i%10) * 0.001),
			Volatility24H: 0.02 + (float64(i%8) * 0.001),
			VolumeZ24H:    float64((i%6)-3) * 0.3,
			RSI14:         50 + (base * 20),
			MACDLine:      base * 2.0,
			MACDSignal:    base * 1.8,
			MACDHist:      base * 0.2,
			BBPos:         0.5 + (base * 0.2),
			BBWidth:       0.05 + mathAbs(base*0.03),
			TargetUp4H:    target,
		})
	}
	return rows
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func nilTracer() trace.Tracer {
	return trace.NewNoopTracerProvider().Tracer("training-test")
}
