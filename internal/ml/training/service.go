package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/ml/common"
	"bug-free-umbrella/internal/ml/features"
	"bug-free-umbrella/internal/ml/models/iforest"
	"bug-free-umbrella/internal/ml/models/logreg"
	"bug-free-umbrella/internal/ml/models/xgboost"

	"go.opentelemetry.io/otel/trace"
)

type FeatureRowStore interface {
	ListLabeledRows(ctx context.Context, interval string, from, to time.Time) ([]domain.MLFeatureRow, error)
	ListRows(ctx context.Context, interval string, from, to time.Time) ([]domain.MLFeatureRow, error)
}

type ModelRegistry interface {
	NextVersion(ctx context.Context, modelKey string) (int, error)
	InsertModelVersion(ctx context.Context, model domain.MLModelVersion) (*domain.MLModelVersion, error)
	GetActiveModel(ctx context.Context, modelKey string) (*domain.MLModelVersion, error)
	ActivateModel(ctx context.Context, modelKey string, version int) error
}

type Config struct {
	Interval          string
	Intervals         []string
	TrainWindowDays   int
	MinTrainSamples   int
	EnableIForest     bool
	IForestTrees      int
	IForestSampleSize int
}

type Service struct {
	tracer   trace.Tracer
	features FeatureRowStore
	registry ModelRegistry
	cfg      Config
}

type ModelTrainResult struct {
	ModelKey     string
	Interval     string
	Version      int
	SampleCount  int
	TestCount    int
	AUC          float64
	Promoted     bool
	PromoteError error
}

func NewService(tracer trace.Tracer, features FeatureRowStore, registry ModelRegistry, cfg Config) *Service {
	if cfg.Interval == "" {
		cfg.Interval = "1h"
	}
	if len(cfg.Intervals) == 0 {
		cfg.Intervals = []string{cfg.Interval}
	}
	if cfg.TrainWindowDays <= 0 {
		cfg.TrainWindowDays = 90
	}
	if cfg.MinTrainSamples <= 0 {
		cfg.MinTrainSamples = 1000
	}
	if cfg.IForestTrees <= 0 {
		cfg.IForestTrees = iforest.DefaultTrainOptions().NumTrees
	}
	if cfg.IForestSampleSize <= 0 {
		cfg.IForestSampleSize = iforest.DefaultTrainOptions().SampleSize
	}
	return &Service{tracer: tracer, features: features, registry: registry, cfg: cfg}
}

func (s *Service) TrainAll(ctx context.Context, now time.Time) ([]ModelTrainResult, error) {
	_, span := s.tracer.Start(ctx, "ml-training.train-all")
	defer span.End()

	from := now.UTC().AddDate(0, 0, -s.cfg.TrainWindowDays)
	results := make([]ModelTrainResult, 0, 4)

	directionalResults, err := s.trainDirectional(ctx, from, now.UTC())
	if err != nil {
		return nil, err
	}
	results = append(results, directionalResults...)

	if s.cfg.EnableIForest {
		anomalyResults, err := s.trainAnomaly(ctx, from, now.UTC())
		if err != nil {
			return nil, err
		}
		results = append(results, anomalyResults...)
	}

	return results, nil
}

func (s *Service) trainDirectional(ctx context.Context, from, now time.Time) ([]ModelTrainResult, error) {
	rows, err := s.features.ListLabeledRows(ctx, s.cfg.Interval, from, now)
	if err != nil {
		return nil, err
	}
	samples, labels := buildDataset(rows)
	if len(samples) < s.cfg.MinTrainSamples {
		return nil, fmt.Errorf("not enough labeled samples: got %d need >= %d", len(samples), s.cfg.MinTrainSamples)
	}

	trainX, trainY, _, _, testX, testY := chronologicalSplit(samples, labels)
	if len(trainX) == 0 || len(testX) == 0 {
		return nil, errors.New("dataset split produced empty partitions")
	}

	results := make([]ModelTrainResult, 0, 2)

	lrOpts := logreg.DefaultTrainOptions()
	lrModel, err := logreg.Train(trainX, trainY, common.FeatureNames, lrOpts)
	if err != nil {
		return nil, fmt.Errorf("train logreg: %w", err)
	}
	lrBlob, err := lrModel.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal logreg model: %w", err)
	}
	lrPreds := lrModel.PredictBatch(testX)
	lrMetrics := computeMetrics(testY, lrPreds)
	lrResult, err := s.persistAndMaybePromote(ctx, common.ModelKeyLogReg, s.cfg.Interval, now, from, lrBlob, "json/logreg-v1", map[string]any{
		"learning_rate": lrOpts.LearningRate,
		"epochs":        lrOpts.Epochs,
		"l2":            lrOpts.L2,
	}, lrMetrics, len(samples), len(testY))
	if err != nil {
		return nil, err
	}
	results = append(results, lrResult)

	xgbOpts := xgboost.DefaultTrainOptions()
	xgbModel, err := xgboost.Train(trainX, trainY, common.FeatureNames, xgbOpts)
	if err != nil {
		return nil, fmt.Errorf("train xgboost: %w", err)
	}
	xgbBlob, err := xgbModel.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal xgboost model: %w", err)
	}
	xgbPreds := xgbModel.PredictBatch(testX)
	xgbMetrics := computeMetrics(testY, xgbPreds)
	xgbResult, err := s.persistAndMaybePromote(ctx, common.ModelKeyXGBoost, s.cfg.Interval, now, from, xgbBlob, "json/boo-xgboost-v1", map[string]any{
		"rounds":        xgbOpts.Rounds,
		"learning_rate": xgbOpts.LearningRate,
		"max_depth":     xgbOpts.MaxDepth,
	}, xgbMetrics, len(samples), len(testY))
	if err != nil {
		return nil, err
	}
	results = append(results, xgbResult)

	return results, nil
}

func (s *Service) trainAnomaly(ctx context.Context, from, now time.Time) ([]ModelTrainResult, error) {
	intervals := uniqueIntervals(s.cfg.Intervals, s.cfg.Interval)
	results := make([]ModelTrainResult, 0, len(intervals))
	minSamples := s.minAnomalySamples()

	for _, interval := range intervals {
		rows, err := s.features.ListRows(ctx, interval, from, now)
		if err != nil {
			return nil, err
		}
		samples := buildAnomalyDataset(rows)
		if len(samples) < minSamples {
			continue
		}
		modelKey := common.IForestModelKey(interval)
		model, err := iforest.Train(samples, common.FeatureNames, modelKey, interval, from, now, iforest.TrainOptions{
			NumTrees:   s.cfg.IForestTrees,
			SampleSize: s.cfg.IForestSampleSize,
		})
		if err != nil {
			return nil, fmt.Errorf("train %s: %w", modelKey, err)
		}
		blob, err := model.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", modelKey, err)
		}
		scores := model.PredictBatch(samples)
		metrics := anomalyMetrics(scores)
		result, err := s.persistAndMaybePromoteAnomaly(
			ctx,
			modelKey,
			interval,
			now,
			from,
			blob,
			map[string]any{
				"num_trees":   s.cfg.IForestTrees,
				"sample_size": s.cfg.IForestSampleSize,
			},
			metrics,
			len(samples),
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (s *Service) minAnomalySamples() int {
	minSamples := s.cfg.MinTrainSamples / 2
	if minSamples < 300 {
		minSamples = 300
	}
	return minSamples
}

func (s *Service) persistAndMaybePromote(
	ctx context.Context,
	modelKey string,
	interval string,
	now time.Time,
	trainedFrom time.Time,
	artifact []byte,
	artifactFormat string,
	hyperparams map[string]any,
	metrics map[string]float64,
	sampleCount int,
	testCount int,
) (ModelTrainResult, error) {
	version, err := s.registry.NextVersion(ctx, modelKey)
	if err != nil {
		return ModelTrainResult{}, err
	}
	hyperJSON, _ := json.Marshal(hyperparams)
	metricJSON, _ := json.Marshal(metrics)

	inserted, err := s.registry.InsertModelVersion(ctx, domain.MLModelVersion{
		ModelKey:           modelKey,
		Version:            version,
		FeatureSpecVersion: features.FeatureSpecVersion(),
		TrainedFrom:        trainedFrom,
		TrainedTo:          now,
		HyperparamsJSON:    string(hyperJSON),
		MetricsJSON:        string(metricJSON),
		ArtifactFormat:     artifactFormat,
		ArtifactBlob:       artifact,
		IsActive:           false,
	})
	if err != nil {
		return ModelTrainResult{}, err
	}

	result := ModelTrainResult{
		ModelKey:    modelKey,
		Interval:    interval,
		Version:     inserted.Version,
		SampleCount: sampleCount,
		TestCount:   testCount,
		AUC:         metrics["auc"],
	}

	promote, promoteErr := s.shouldPromote(ctx, modelKey, metrics["auc"], testCount, inserted.Version)
	if promoteErr != nil {
		result.PromoteError = promoteErr
		return result, nil
	}
	if promote {
		if err := s.registry.ActivateModel(ctx, modelKey, inserted.Version); err != nil {
			result.PromoteError = err
			return result, nil
		}
		result.Promoted = true
	}
	return result, nil
}

func (s *Service) persistAndMaybePromoteAnomaly(
	ctx context.Context,
	modelKey string,
	interval string,
	now time.Time,
	trainedFrom time.Time,
	artifact []byte,
	hyperparams map[string]any,
	metrics map[string]float64,
	sampleCount int,
) (ModelTrainResult, error) {
	version, err := s.registry.NextVersion(ctx, modelKey)
	if err != nil {
		return ModelTrainResult{}, err
	}
	hyperJSON, _ := json.Marshal(hyperparams)
	metricJSON, _ := json.Marshal(metrics)

	inserted, err := s.registry.InsertModelVersion(ctx, domain.MLModelVersion{
		ModelKey:           modelKey,
		Version:            version,
		FeatureSpecVersion: features.FeatureSpecVersion(),
		TrainedFrom:        trainedFrom,
		TrainedTo:          now,
		HyperparamsJSON:    string(hyperJSON),
		MetricsJSON:        string(metricJSON),
		ArtifactFormat:     "json/iforest-v1",
		ArtifactBlob:       artifact,
		IsActive:           false,
	})
	if err != nil {
		return ModelTrainResult{}, err
	}

	result := ModelTrainResult{
		ModelKey:    modelKey,
		Interval:    interval,
		Version:     inserted.Version,
		SampleCount: sampleCount,
	}

	promote, promoteErr := s.shouldPromoteAnomaly(ctx, modelKey, metrics["score_std"], inserted.Version)
	if promoteErr != nil {
		result.PromoteError = promoteErr
		return result, nil
	}
	if promote {
		if err := s.registry.ActivateModel(ctx, modelKey, inserted.Version); err != nil {
			result.PromoteError = err
			return result, nil
		}
		result.Promoted = true
	}
	return result, nil
}

func (s *Service) shouldPromote(ctx context.Context, modelKey string, newAUC float64, testCount int, newVersion int) (bool, error) {
	active, err := s.registry.GetActiveModel(ctx, modelKey)
	if err != nil {
		return false, err
	}
	if active == nil {
		return true, nil
	}
	if active.Version == newVersion {
		return active.IsActive, nil
	}
	if testCount < 300 {
		return false, nil
	}
	activeAUC, ok := metricValue(active.MetricsJSON, "auc")
	if !ok {
		return true, nil
	}
	return newAUC >= activeAUC+0.01, nil
}

func (s *Service) shouldPromoteAnomaly(ctx context.Context, modelKey string, newStd float64, newVersion int) (bool, error) {
	active, err := s.registry.GetActiveModel(ctx, modelKey)
	if err != nil {
		return false, err
	}
	if active == nil {
		return true, nil
	}
	if active.Version == newVersion {
		return active.IsActive, nil
	}
	activeStd, ok := metricValue(active.MetricsJSON, "score_std")
	if !ok {
		return true, nil
	}
	return newStd >= activeStd+0.01, nil
}

func buildDataset(rows []domain.MLFeatureRow) ([][]float64, []float64) {
	x := make([][]float64, 0, len(rows))
	y := make([]float64, 0, len(rows))
	for i := range rows {
		label, ok := common.TargetLabel(rows[i])
		if !ok {
			continue
		}
		x = append(x, common.FeatureVector(rows[i]))
		y = append(y, label)
	}
	return x, y
}

func buildAnomalyDataset(rows []domain.MLFeatureRow) [][]float64 {
	x := make([][]float64, 0, len(rows))
	for i := range rows {
		x = append(x, common.FeatureVector(rows[i]))
	}
	return x
}

func chronologicalSplit(samples [][]float64, labels []float64) (trainX [][]float64, trainY []float64, valX [][]float64, valY []float64, testX [][]float64, testY []float64) {
	n := len(samples)
	if n == 0 {
		return nil, nil, nil, nil, nil, nil
	}
	trainEnd := int(float64(n) * 0.70)
	valEnd := int(float64(n) * 0.85)
	if trainEnd <= 0 {
		trainEnd = n / 2
	}
	if valEnd <= trainEnd {
		valEnd = (trainEnd + n) / 2
	}
	if valEnd >= n {
		valEnd = n - 1
	}
	if valEnd <= trainEnd {
		trainEnd = n - 2
		valEnd = n - 1
	}
	if trainEnd < 1 {
		trainEnd = 1
	}
	if valEnd < trainEnd+1 {
		valEnd = trainEnd + 1
	}
	if valEnd >= n {
		valEnd = n - 1
	}
	return samples[:trainEnd], labels[:trainEnd],
		samples[trainEnd:valEnd], labels[trainEnd:valEnd],
		samples[valEnd:], labels[valEnd:]
}

func anomalyMetrics(scores []float64) map[string]float64 {
	if len(scores) == 0 {
		return map[string]float64{
			"score_mean": 0,
			"score_std":  0,
			"score_p95":  0,
			"n":          0,
		}
	}
	values := make([]float64, len(scores))
	for i, score := range scores {
		values[i] = common.Clamp01(score)
	}
	mean, std := meanStd(values)
	return map[string]float64{
		"score_mean": mean,
		"score_std":  std,
		"score_p95":  percentile(values, 0.95),
		"n":          float64(len(values)),
	}
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	if len(values) == 1 {
		return mean, 0
	}
	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if p <= 0 {
		p = 0
	}
	if p >= 1 {
		p = 1
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Round(p * float64(len(sorted)-1)))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func metricValue(metricsJSON, key string) (float64, bool) {
	var m map[string]float64
	if err := json.Unmarshal([]byte(metricsJSON), &m); err != nil {
		return 0, false
	}
	v, ok := m[key]
	return v, ok
}

func computeMetrics(labels []float64, probs []float64) map[string]float64 {
	n := len(labels)
	if n == 0 || len(probs) != n {
		return map[string]float64{"auc": 0.5, "accuracy": 0, "precision": 0, "recall": 0, "f1": 0, "brier": 0, "n_test": 0}
	}
	tp := 0.0
	fp := 0.0
	tn := 0.0
	fn := 0.0
	brier := 0.0
	for i := 0; i < n; i++ {
		y := labels[i]
		p := common.Clamp01(probs[i])
		pred := 0.0
		if p >= 0.5 {
			pred = 1
		}
		if pred == 1 && y == 1 {
			tp++
		}
		if pred == 1 && y == 0 {
			fp++
		}
		if pred == 0 && y == 0 {
			tn++
		}
		if pred == 0 && y == 1 {
			fn++
		}
		d := p - y
		brier += d * d
	}

	accuracy := (tp + tn) / float64(n)
	precision := 0.0
	if tp+fp > 0 {
		precision = tp / (tp + fp)
	}
	recall := 0.0
	if tp+fn > 0 {
		recall = tp / (tp + fn)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	auc := computeAUC(labels, probs)
	return map[string]float64{
		"auc":       auc,
		"accuracy":  accuracy,
		"precision": precision,
		"recall":    recall,
		"f1":        f1,
		"brier":     brier / float64(n),
		"n_test":    float64(n),
	}
}

func computeAUC(labels []float64, probs []float64) float64 {
	type pair struct {
		p float64
		y float64
	}
	pairs := make([]pair, len(labels))
	pos := 0.0
	neg := 0.0
	for i := range labels {
		pairs[i] = pair{p: common.Clamp01(probs[i]), y: labels[i]}
		if labels[i] >= 0.5 {
			pos++
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return 0.5
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].p < pairs[j].p })

	sumRankPos := 0.0
	rank := 1.0
	for i := 0; i < len(pairs); {
		j := i + 1
		for j < len(pairs) && math.Abs(pairs[j].p-pairs[i].p) < 1e-12 {
			j++
		}
		avgRank := (rank + float64(j)) / 2
		for k := i; k < j; k++ {
			if pairs[k].y >= 0.5 {
				sumRankPos += avgRank
			}
		}
		rank = float64(j + 1)
		i = j
	}
	auc := (sumRankPos - (pos*(pos+1))/2) / (pos * neg)
	if math.IsNaN(auc) || math.IsInf(auc, 0) {
		return 0.5
	}
	return auc
}

func uniqueIntervals(intervals []string, fallback string) []string {
	if fallback == "" {
		fallback = "1h"
	}
	if len(intervals) == 0 {
		return []string{fallback}
	}
	seen := make(map[string]struct{}, len(intervals))
	out := make([]string, 0, len(intervals))
	for _, interval := range intervals {
		if interval == "" {
			continue
		}
		if _, ok := seen[interval]; ok {
			continue
		}
		seen[interval] = struct{}{}
		out = append(out, interval)
	}
	if len(out) == 0 {
		return []string{fallback}
	}
	return out
}
