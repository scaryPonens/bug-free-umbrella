package iforest

import (
	"math"
	"testing"
	"time"
)

func TestTrainPredictAndRoundTrip(t *testing.T) {
	samples := dataset()
	model, err := Train(
		samples,
		[]string{"x1", "x2"},
		"iforest_1h",
		"1h",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		TrainOptions{NumTrees: 100, SampleSize: 64},
	)
	if err != nil {
		t.Fatalf("train failed: %v", err)
	}

	normalScore := model.PredictScore([]float64{0.12, 0.03})
	anomalyScore := model.PredictScore([]float64{6.5, 6.8})
	if normalScore < 0 || normalScore > 1 || anomalyScore < 0 || anomalyScore > 1 {
		t.Fatalf("expected scores in [0,1], got normal=%.4f anomaly=%.4f", normalScore, anomalyScore)
	}
	if anomalyScore <= normalScore {
		t.Fatalf("expected anomaly score > normal score, got normal=%.4f anomaly=%.4f", normalScore, anomalyScore)
	}

	blob, err := model.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	restored, err := UnmarshalBinary(blob)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	pRestored := restored.PredictScore([]float64{6.5, 6.8})
	if pRestored < 0 || pRestored > 1 {
		t.Fatalf("expected restored score in [0,1], got %.4f", pRestored)
	}
	if diff := math.Abs(anomalyScore - pRestored); diff > 1e-9 {
		t.Fatalf("roundtrip changed anomaly score by %.10f", diff)
	}
}

func dataset() [][]float64 {
	out := make([][]float64, 0, 120)
	for i := 0; i < 60; i++ {
		out = append(out, []float64{
			-0.2 + float64(i)/300.0,
			0.1 + float64(i)/500.0,
		})
	}
	for i := 0; i < 60; i++ {
		out = append(out, []float64{
			0.3 + float64(i)/300.0,
			-0.15 + float64(i)/500.0,
		})
	}
	return out
}
