package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/ml/common"
	"bug-free-umbrella/internal/ml/ensemble"
	iforestmodel "bug-free-umbrella/internal/ml/models/iforest"
	"bug-free-umbrella/internal/ml/models/logreg"
	"bug-free-umbrella/internal/ml/models/xgboost"

	"go.opentelemetry.io/otel/trace"
)

type FeatureReader interface {
	ListLatestByInterval(ctx context.Context, interval string) ([]domain.MLFeatureRow, error)
}

type ModelRegistry interface {
	GetActiveModel(ctx context.Context, modelKey string) (*domain.MLModelVersion, error)
}

type PredictionStore interface {
	UpsertPrediction(ctx context.Context, prediction domain.MLPrediction) (*domain.MLPrediction, error)
	AttachSignalID(ctx context.Context, predictionID, signalID int64) error
}

type SignalStore interface {
	InsertSignals(ctx context.Context, signals []domain.Signal) ([]domain.Signal, error)
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

type Config struct {
	Interval         string
	Intervals        []string
	TargetHours      int
	LongThreshold    float64
	ShortThreshold   float64
	EnableIForest    bool
	AnomalyThreshold float64
	AnomalyDampMax   float64
}

type Service struct {
	tracer      trace.Tracer
	features    FeatureReader
	registry    ModelRegistry
	predictions PredictionStore
	signals     SignalStore
	ensemble    *ensemble.Service
	cfg         Config
}

type RunResult struct {
	Predictions int
	Signals     int
}

func NewService(
	tracer trace.Tracer,
	features FeatureReader,
	registry ModelRegistry,
	predictions PredictionStore,
	signals SignalStore,
	ensembleSvc *ensemble.Service,
	cfg Config,
) *Service {
	if cfg.Interval == "" {
		cfg.Interval = "1h"
	}
	if len(cfg.Intervals) == 0 {
		cfg.Intervals = []string{cfg.Interval}
	}
	if cfg.TargetHours <= 0 {
		cfg.TargetHours = 4
	}
	if cfg.LongThreshold <= 0 || cfg.LongThreshold >= 1 {
		cfg.LongThreshold = 0.55
	}
	if cfg.ShortThreshold <= 0 || cfg.ShortThreshold >= 1 {
		cfg.ShortThreshold = 0.45
	}
	if cfg.AnomalyThreshold <= 0 || cfg.AnomalyThreshold >= 1 {
		cfg.AnomalyThreshold = 0.62
	}
	if cfg.AnomalyDampMax < 0 || cfg.AnomalyDampMax > 1 {
		cfg.AnomalyDampMax = 0.65
	}
	if ensembleSvc == nil {
		ensembleSvc = ensemble.NewService()
	}
	return &Service{
		tracer:      tracer,
		features:    features,
		registry:    registry,
		predictions: predictions,
		signals:     signals,
		ensemble:    ensembleSvc,
		cfg:         cfg,
	}
}

func (s *Service) RunLatest(ctx context.Context, now time.Time) (RunResult, error) {
	_, span := s.tracer.Start(ctx, "ml-inference.run-latest")
	defer span.End()

	if s.features == nil || s.registry == nil || s.predictions == nil || s.signals == nil {
		return RunResult{}, fmt.Errorf("ml inference service is not fully initialized")
	}

	logVersion, logPredict, err := s.loadLogReg(ctx)
	if err != nil {
		return RunResult{}, err
	}
	xgbVersion, xgbPredict, err := s.loadXGBoost(ctx)
	if err != nil {
		return RunResult{}, err
	}

	result := RunResult{}
	intervals := uniqueIntervals(s.cfg.Intervals, s.cfg.Interval)
	for _, interval := range intervals {
		rows, err := s.features.ListLatestByInterval(ctx, interval)
		if err != nil {
			return result, err
		}
		if len(rows) == 0 {
			continue
		}

		iforestVersion, iforestPredict, err := s.loadIForest(ctx, interval)
		if err != nil {
			return result, err
		}

		for i := range rows {
			row := rows[i]
			targetTime := row.OpenTime.UTC().Add(time.Duration(s.cfg.TargetHours) * time.Hour)
			features := common.FeatureVector(row)
			anomalyScore := 0.0
			dampFactor := 1.0

			if iforestPredict != nil {
				anomalyScore = common.Clamp01(iforestPredict(features))
				dampFactor = s.dampFactor(anomalyScore)
				pred, err := s.persistAnomalyPrediction(ctx, row, iforestVersion, anomalyScore, targetTime, dampFactor)
				if err != nil {
					return result, err
				}
				if pred != nil {
					result.Predictions++
				}
			}

			if row.Interval != s.cfg.Interval || (logPredict == nil && xgbPredict == nil) {
				continue
			}

			classicScore := s.classicScore(ctx, row)
			logProb := 0.5
			xgbProb := 0.5

			if logPredict != nil {
				logProb = common.Clamp01(logPredict(features))
				pred, hasSignal, err := s.persistModelPrediction(ctx, row, common.ModelKeyLogReg, logVersion, logProb, targetTime, 0, anomalyScore, dampFactor)
				if err != nil {
					return result, err
				}
				if pred != nil {
					result.Predictions++
				}
				if hasSignal {
					result.Signals++
				}
			}

			if xgbPredict != nil {
				xgbProb = common.Clamp01(xgbPredict(features))
				pred, hasSignal, err := s.persistModelPrediction(ctx, row, common.ModelKeyXGBoost, xgbVersion, xgbProb, targetTime, 0, anomalyScore, dampFactor)
				if err != nil {
					return result, err
				}
				if pred != nil {
					result.Predictions++
				}
				if hasSignal {
					result.Signals++
				}
			}

			ensembleScore := s.ensemble.Score(ensemble.Components{
				ClassicScore: classicScore,
				LogRegProb:   logProb,
				XGBoostProb:  xgbProb,
			})
			ensembleScore *= dampFactor
			if ensembleScore > 1 {
				ensembleScore = 1
			}
			if ensembleScore < -1 {
				ensembleScore = -1
			}
			ensembleProb := common.Clamp01((ensembleScore + 1) / 2)
			version := max(logVersion, xgbVersion)
			if version <= 0 {
				version = 1
			}
			pred, hasSignal, err := s.persistModelPrediction(ctx, row, common.ModelKeyEnsembleV1, version, ensembleProb, targetTime, ensembleScore, anomalyScore, dampFactor)
			if err != nil {
				return result, err
			}
			if pred != nil {
				result.Predictions++
			}
			if hasSignal {
				result.Signals++
			}
		}
	}

	return result, nil
}

func (s *Service) persistModelPrediction(
	ctx context.Context,
	row domain.MLFeatureRow,
	modelKey string,
	modelVersion int,
	probUp float64,
	targetTime time.Time,
	ensembleScore float64,
	anomalyScore float64,
	dampFactor float64,
) (*domain.MLPrediction, bool, error) {
	confidence := common.Confidence(probUp)
	direction := common.DirectionFromProb(probUp, s.cfg.LongThreshold, s.cfg.ShortThreshold)
	if modelKey == common.ModelKeyEnsembleV1 {
		direction = ensemble.Direction(ensembleScore)
	}
	risk := common.RiskFromConfidence(confidence)
	if modelKey == common.ModelKeyEnsembleV1 && anomalyScore >= s.cfg.AnomalyThreshold {
		risk = riskBump(risk, 1)
	}
	detailsJSON := s.buildDetailsJSON(modelKey, modelVersion, probUp, confidence, ensembleScore, anomalyScore, dampFactor)

	pred, err := s.predictions.UpsertPrediction(ctx, domain.MLPrediction{
		Symbol:       row.Symbol,
		Interval:     row.Interval,
		OpenTime:     row.OpenTime.UTC(),
		TargetTime:   targetTime.UTC(),
		ModelKey:     modelKey,
		ModelVersion: modelVersion,
		ProbUp:       probUp,
		Confidence:   confidence,
		Direction:    direction,
		Risk:         risk,
		DetailsJSON:  detailsJSON,
	})
	if err != nil {
		return nil, false, err
	}

	if direction == domain.DirectionHold {
		return pred, false, nil
	}
	indicator := indicatorForModelKey(modelKey)
	signalDetails := signalDetails(modelKey, modelVersion, probUp, confidence, ensembleScore, anomalyScore, dampFactor)
	persistedSignals, err := s.signals.InsertSignals(ctx, []domain.Signal{{
		Symbol:    row.Symbol,
		Interval:  row.Interval,
		Indicator: indicator,
		Timestamp: row.OpenTime.UTC(),
		Risk:      risk,
		Direction: direction,
		Details:   signalDetails,
	}})
	if err != nil {
		return pred, false, err
	}
	if len(persistedSignals) > 0 && persistedSignals[0].ID > 0 {
		if err := s.predictions.AttachSignalID(ctx, pred.ID, persistedSignals[0].ID); err != nil {
			return pred, false, err
		}
	}
	return pred, true, nil
}

func (s *Service) persistAnomalyPrediction(
	ctx context.Context,
	row domain.MLFeatureRow,
	modelVersion int,
	anomalyScore float64,
	targetTime time.Time,
	dampFactor float64,
) (*domain.MLPrediction, error) {
	risk := riskFromAnomalyScore(anomalyScore)
	detailsJSON := s.buildAnomalyDetailsJSON(row.Interval, modelVersion, anomalyScore, dampFactor)

	return s.predictions.UpsertPrediction(ctx, domain.MLPrediction{
		Symbol:       row.Symbol,
		Interval:     row.Interval,
		OpenTime:     row.OpenTime.UTC(),
		TargetTime:   targetTime.UTC(),
		ModelKey:     common.IForestModelKey(row.Interval),
		ModelVersion: modelVersion,
		ProbUp:       0.5,
		Confidence:   anomalyScore,
		Direction:    domain.DirectionHold,
		Risk:         risk,
		DetailsJSON:  detailsJSON,
	})
}

func (s *Service) loadLogReg(ctx context.Context) (int, func([]float64) float64, error) {
	active, err := s.registry.GetActiveModel(ctx, common.ModelKeyLogReg)
	if err != nil || active == nil {
		return 0, nil, err
	}
	model, err := logreg.UnmarshalBinary(active.ArtifactBlob)
	if err != nil {
		return 0, nil, err
	}
	return active.Version, model.PredictProb, nil
}

func (s *Service) loadXGBoost(ctx context.Context) (int, func([]float64) float64, error) {
	active, err := s.registry.GetActiveModel(ctx, common.ModelKeyXGBoost)
	if err != nil || active == nil {
		return 0, nil, err
	}
	model, err := xgboost.UnmarshalBinary(active.ArtifactBlob)
	if err != nil {
		return 0, nil, err
	}
	return active.Version, model.PredictProb, nil
}

func (s *Service) loadIForest(ctx context.Context, interval string) (int, func([]float64) float64, error) {
	if !s.cfg.EnableIForest {
		return 0, nil, nil
	}
	active, err := s.registry.GetActiveModel(ctx, common.IForestModelKey(interval))
	if err != nil || active == nil {
		return 0, nil, err
	}
	model, err := iforestmodel.UnmarshalBinary(active.ArtifactBlob)
	if err != nil {
		return 0, nil, err
	}
	return active.Version, model.PredictScore, nil
}

func (s *Service) classicScore(ctx context.Context, row domain.MLFeatureRow) float64 {
	signals, err := s.signals.ListSignals(ctx, domain.SignalFilter{Symbol: row.Symbol, Limit: 100})
	if err != nil {
		return 0
	}
	targetTS := row.OpenTime.UTC().Unix()
	weighted := 0.0
	weightTotal := 0.0
	for i := range signals {
		sig := signals[i]
		if sig.Interval != row.Interval || sig.Timestamp.UTC().Unix() != targetTS {
			continue
		}
		if !isClassicIndicator(sig.Indicator) {
			continue
		}
		dir := 0.0
		switch sig.Direction {
		case domain.DirectionLong:
			dir = 1
		case domain.DirectionShort:
			dir = -1
		default:
			dir = 0
		}
		weight := (6.0 - float64(sig.Risk)) / 5.0
		if weight < 0 {
			weight = 0
		}
		weighted += dir * weight
		weightTotal += weight
	}
	if weightTotal == 0 {
		return 0
	}
	score := weighted / weightTotal
	if score > 1 {
		return 1
	}
	if score < -1 {
		return -1
	}
	return score
}

func (s *Service) buildDetailsJSON(modelKey string, version int, probUp, confidence, ensembleScore, anomalyScore, dampFactor float64) string {
	payload := map[string]any{
		"model_key":     modelKey,
		"model_version": version,
		"prob_up":       roundFloat(probUp),
		"confidence":    roundFloat(confidence),
		"target":        "4h",
	}
	if modelKey == common.ModelKeyEnsembleV1 {
		payload["ensemble_score"] = roundFloat(ensembleScore)
	}
	if anomalyScore > 0 {
		payload["anomaly_score"] = roundFloat(anomalyScore)
		payload["damp_factor"] = roundFloat(dampFactor)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *Service) buildAnomalyDetailsJSON(interval string, version int, anomalyScore, dampFactor float64) string {
	payload := map[string]any{
		"model_key":     common.IForestModelKey(interval),
		"model_version": version,
		"anomaly_score": roundFloat(anomalyScore),
		"threshold":     roundFloat(s.cfg.AnomalyThreshold),
		"damp_factor":   roundFloat(dampFactor),
		"target":        "4h",
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func signalDetails(modelKey string, version int, probUp, confidence, ensembleScore, anomalyScore, dampFactor float64) string {
	if modelKey == common.ModelKeyEnsembleV1 {
		if anomalyScore > 0 {
			return fmt.Sprintf(
				"model_key=%s;model_version=%d;prob_up=%.4f;confidence=%.4f;target=4h;ensemble_score=%.4f;anomaly_score=%.4f;damp_factor=%.4f",
				modelKey, version, probUp, confidence, ensembleScore, anomalyScore, dampFactor,
			)
		}
		return fmt.Sprintf(
			"model_key=%s;model_version=%d;prob_up=%.4f;confidence=%.4f;target=4h;ensemble_score=%.4f",
			modelKey, version, probUp, confidence, ensembleScore,
		)
	}
	return fmt.Sprintf(
		"model_key=%s;model_version=%d;prob_up=%.4f;confidence=%.4f;target=4h",
		modelKey, version, probUp, confidence,
	)
}

func indicatorForModelKey(modelKey string) string {
	switch modelKey {
	case common.ModelKeyLogReg:
		return domain.IndicatorMLLogRegUp4H
	case common.ModelKeyXGBoost:
		return domain.IndicatorMLXGBoostUp4H
	default:
		return domain.IndicatorMLEnsembleUp4H
	}
}

func isClassicIndicator(indicator string) bool {
	switch indicator {
	case domain.IndicatorRSI, domain.IndicatorMACD, domain.IndicatorBollinger, domain.IndicatorVolumeZ:
		return true
	default:
		return false
	}
}

func (s *Service) dampFactor(anomalyScore float64) float64 {
	factor := 1 - (s.cfg.AnomalyDampMax * common.Clamp01(anomalyScore))
	if factor < 0 {
		return 0
	}
	if factor > 1 {
		return 1
	}
	return factor
}

func riskFromAnomalyScore(score float64) domain.RiskLevel {
	score = common.Clamp01(score)
	switch {
	case score >= 0.9:
		return domain.RiskLevel2
	case score >= 0.75:
		return domain.RiskLevel3
	case score >= 0.6:
		return domain.RiskLevel4
	default:
		return domain.RiskLevel5
	}
}

func riskBump(risk domain.RiskLevel, delta int) domain.RiskLevel {
	next := int(risk) + delta
	if next > int(domain.RiskLevel5) {
		return domain.RiskLevel5
	}
	if next < int(domain.RiskLevel1) {
		return domain.RiskLevel1
	}
	return domain.RiskLevel(next)
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

func roundFloat(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10000) / 10000
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
