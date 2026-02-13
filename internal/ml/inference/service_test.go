package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/ml/common"
	iforestmodel "bug-free-umbrella/internal/ml/models/iforest"
	"bug-free-umbrella/internal/ml/models/logreg"
	"bug-free-umbrella/internal/ml/models/xgboost"

	"go.opentelemetry.io/otel/trace"
)

func TestRunLatestPersistsAnomalyAndSkipsAnomalySignals(t *testing.T) {
	rowTS := time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)
	features := &featureReaderStub{
		byInterval: map[string][]domain.MLFeatureRow{
			"1h": {makeFeatureRow("BTC", "1h", rowTS, 2.5)},
			"4h": {makeFeatureRow("BTC", "4h", rowTS, 2.8)},
		},
	}

	logModelBlob := mustTrainLogRegBlob(t)
	xgbModelBlob := mustTrainXGBBlob(t)
	iforest1hBlob := mustTrainIForestBlob(t, "iforest_1h", "1h")
	iforest4hBlob := mustTrainIForestBlob(t, "iforest_4h", "4h")

	registry := &modelRegistryStub{
		active: map[string]*domain.MLModelVersion{
			common.ModelKeyLogReg:        {ModelKey: common.ModelKeyLogReg, Version: 1, ArtifactBlob: logModelBlob, IsActive: true},
			common.ModelKeyXGBoost:       {ModelKey: common.ModelKeyXGBoost, Version: 1, ArtifactBlob: xgbModelBlob, IsActive: true},
			common.IForestModelKey("1h"): {ModelKey: common.IForestModelKey("1h"), Version: 1, ArtifactBlob: iforest1hBlob, IsActive: true},
			common.IForestModelKey("4h"): {ModelKey: common.IForestModelKey("4h"), Version: 1, ArtifactBlob: iforest4hBlob, IsActive: true},
		},
	}
	predictions := newPredictionStoreStub()
	signals := &signalStoreStub{
		classicSignals: []domain.Signal{
			{
				Symbol:    "BTC",
				Interval:  "1h",
				Indicator: domain.IndicatorRSI,
				Timestamp: rowTS,
				Risk:      domain.RiskLevel3,
				Direction: domain.DirectionLong,
			},
		},
	}

	svc := NewService(
		trace.NewNoopTracerProvider().Tracer("inference-test"),
		features,
		registry,
		predictions,
		signals,
		nil,
		Config{
			Interval:         "1h",
			Intervals:        []string{"1h", "4h"},
			TargetHours:      4,
			LongThreshold:    0.55,
			ShortThreshold:   0.45,
			EnableIForest:    true,
			AnomalyThreshold: 0.20,
			AnomalyDampMax:   0.65,
		},
	)

	result, err := svc.RunLatest(context.Background(), rowTS.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("run latest failed: %v", err)
	}
	if result.Predictions != 5 {
		t.Fatalf("expected 5 predictions (2 anomaly + 3 directional), got %d", result.Predictions)
	}
	if result.Signals == 0 {
		t.Fatal("expected directional signals to be inserted")
	}

	iforest1h := predictions.findByKey(common.IForestModelKey("1h"), "1h")
	if iforest1h == nil {
		t.Fatal("expected iforest 1h prediction")
	}
	if iforest1h.Direction != domain.DirectionHold || iforest1h.SignalID != nil {
		t.Fatalf("iforest prediction should be hold with no signal id, got direction=%s signal_id=%v", iforest1h.Direction, iforest1h.SignalID)
	}

	iforest4h := predictions.findByKey(common.IForestModelKey("4h"), "4h")
	if iforest4h == nil {
		t.Fatal("expected iforest 4h prediction")
	}
	if iforest4h.Direction != domain.DirectionHold || iforest4h.SignalID != nil {
		t.Fatalf("iforest 4h prediction should be hold with no signal id, got direction=%s signal_id=%v", iforest4h.Direction, iforest4h.SignalID)
	}

	for _, sig := range signals.inserted {
		if strings.HasPrefix(sig.Indicator, "iforest") {
			t.Fatalf("anomaly should not emit standalone signals: %+v", sig)
		}
	}

	ensemblePred := predictions.findByKey(common.ModelKeyEnsembleV1, "1h")
	if ensemblePred == nil {
		t.Fatal("missing ensemble prediction")
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(ensemblePred.DetailsJSON), &details); err != nil {
		t.Fatalf("failed to parse details: %v", err)
	}
	if _, ok := details["anomaly_score"]; !ok {
		t.Fatalf("expected anomaly_score in ensemble details: %s", ensemblePred.DetailsJSON)
	}
	if _, ok := details["damp_factor"]; !ok {
		t.Fatalf("expected damp_factor in ensemble details: %s", ensemblePred.DetailsJSON)
	}
}

type featureReaderStub struct {
	byInterval map[string][]domain.MLFeatureRow
}

func (s *featureReaderStub) ListLatestByInterval(_ context.Context, interval string) ([]domain.MLFeatureRow, error) {
	return append([]domain.MLFeatureRow(nil), s.byInterval[interval]...), nil
}

type modelRegistryStub struct {
	active map[string]*domain.MLModelVersion
}

func (s *modelRegistryStub) GetActiveModel(_ context.Context, modelKey string) (*domain.MLModelVersion, error) {
	model := s.active[modelKey]
	if model == nil {
		return nil, nil
	}
	copyModel := *model
	return &copyModel, nil
}

type predictionStoreStub struct {
	mu     sync.Mutex
	nextID int64
	rows   map[string]domain.MLPrediction
}

func newPredictionStoreStub() *predictionStoreStub {
	return &predictionStoreStub{
		nextID: 1,
		rows:   make(map[string]domain.MLPrediction),
	}
}

func (s *predictionStoreStub) UpsertPrediction(_ context.Context, prediction domain.MLPrediction) (*domain.MLPrediction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := predictionRowKey(prediction)
	if existing, ok := s.rows[key]; ok {
		prediction.ID = existing.ID
		prediction.SignalID = existing.SignalID
		s.rows[key] = prediction
		copyPred := prediction
		return &copyPred, nil
	}
	prediction.ID = s.nextID
	s.nextID++
	s.rows[key] = prediction
	copyPred := prediction
	return &copyPred, nil
}

func (s *predictionStoreStub) AttachSignalID(_ context.Context, predictionID, signalID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, pred := range s.rows {
		if pred.ID == predictionID {
			sid := signalID
			pred.SignalID = &sid
			s.rows[key] = pred
			return nil
		}
	}
	return fmt.Errorf("prediction id not found: %d", predictionID)
}

func (s *predictionStoreStub) findByKey(modelKey, interval string) *domain.MLPrediction {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pred := range s.rows {
		if pred.ModelKey == modelKey && pred.Interval == interval {
			copyPred := pred
			return &copyPred
		}
	}
	return nil
}

func predictionRowKey(p domain.MLPrediction) string {
	return fmt.Sprintf("%s|%s|%d|%s|%d", p.Symbol, p.Interval, p.OpenTime.UTC().Unix(), p.ModelKey, p.ModelVersion)
}

type signalStoreStub struct {
	mu             sync.Mutex
	nextID         int64
	inserted       []domain.Signal
	classicSignals []domain.Signal
}

func (s *signalStoreStub) InsertSignals(_ context.Context, in []domain.Signal) ([]domain.Signal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextID == 0 {
		s.nextID = 1
	}
	out := make([]domain.Signal, len(in))
	copy(out, in)
	for i := range out {
		out[i].ID = s.nextID
		s.nextID++
	}
	s.inserted = append(s.inserted, out...)
	return out, nil
}

func (s *signalStoreStub) ListSignals(_ context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Signal, 0, len(s.classicSignals))
	for _, sig := range s.classicSignals {
		if filter.Symbol != "" && sig.Symbol != filter.Symbol {
			continue
		}
		out = append(out, sig)
	}
	return out, nil
}

func makeFeatureRow(symbol, interval string, ts time.Time, base float64) domain.MLFeatureRow {
	return domain.MLFeatureRow{
		Symbol:        symbol,
		Interval:      interval,
		OpenTime:      ts,
		Ret1H:         base,
		Ret4H:         base * 0.8,
		Ret12H:        base * 0.6,
		Ret24H:        base * 0.4,
		Volatility6H:  0.05,
		Volatility24H: 0.08,
		VolumeZ24H:    base * 0.5,
		RSI14:         50 + base*10,
		MACDLine:      base,
		MACDSignal:    base * 0.9,
		MACDHist:      base * 0.1,
		BBPos:         0.5 + base*0.1,
		BBWidth:       0.1,
	}
}

func mustTrainLogRegBlob(t *testing.T) []byte {
	t.Helper()
	samples, labels := directionalDataset()
	model, err := logreg.Train(samples, labels, common.FeatureNames, logreg.DefaultTrainOptions())
	if err != nil {
		t.Fatalf("train logreg: %v", err)
	}
	blob, err := model.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal logreg: %v", err)
	}
	return blob
}

func mustTrainXGBBlob(t *testing.T) []byte {
	t.Helper()
	samples, labels := directionalDataset()
	model, err := xgboost.Train(samples, labels, common.FeatureNames, xgboost.DefaultTrainOptions())
	if err != nil {
		t.Fatalf("train xgboost: %v", err)
	}
	blob, err := model.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal xgboost: %v", err)
	}
	return blob
}

func mustTrainIForestBlob(t *testing.T, modelKey, interval string) []byte {
	t.Helper()
	model, err := iforestmodel.Train(
		anomalyDataset(),
		common.FeatureNames,
		modelKey,
		interval,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		iforestmodel.TrainOptions{NumTrees: 120, SampleSize: 64},
	)
	if err != nil {
		t.Fatalf("train iforest: %v", err)
	}
	blob, err := model.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal iforest: %v", err)
	}
	return blob
}

func directionalDataset() ([][]float64, []float64) {
	samples := make([][]float64, 0, 300)
	labels := make([]float64, 0, 300)
	for i := 0; i < 150; i++ {
		row := make([]float64, len(common.FeatureNames))
		for j := range row {
			row[j] = -1.5 + float64(i)/300.0 + float64(j)*0.01
		}
		samples = append(samples, row)
		labels = append(labels, 0)
	}
	for i := 0; i < 150; i++ {
		row := make([]float64, len(common.FeatureNames))
		for j := range row {
			row[j] = 1.5 + float64(i)/300.0 + float64(j)*0.01
		}
		samples = append(samples, row)
		labels = append(labels, 1)
	}
	return samples, labels
}

func anomalyDataset() [][]float64 {
	samples := make([][]float64, 0, 400)
	for i := 0; i < 400; i++ {
		row := make([]float64, len(common.FeatureNames))
		for j := range row {
			row[j] = (float64((i+j)%7) - 3.0) * 0.05
		}
		samples = append(samples, row)
	}
	return samples
}
