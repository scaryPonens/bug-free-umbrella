package iforest

import (
	"encoding/json"
	"errors"
	"math"
	"time"

	goiforest "github.com/narumiruna/go-iforest/pkg/iforest"
)

type TrainOptions struct {
	NumTrees   int
	SampleSize int
}

type Artifact struct {
	ModelKey     string                `json:"model_key"`
	Interval     string                `json:"interval"`
	FeatureNames []string              `json:"feature_names"`
	Means        []float64             `json:"means"`
	Stds         []float64             `json:"stds"`
	Options      goiforest.Options     `json:"options"`
	Trees        []*goiforest.TreeNode `json:"trees"`
	TrainedFrom  time.Time             `json:"trained_from"`
	TrainedTo    time.Time             `json:"trained_to"`
}

type Model struct {
	artifact Artifact
	forest   *goiforest.IsolationForest
}

func DefaultTrainOptions() TrainOptions {
	return TrainOptions{
		NumTrees:   200,
		SampleSize: 256,
	}
}

func Train(
	samples [][]float64,
	featureNames []string,
	modelKey string,
	interval string,
	trainedFrom, trainedTo time.Time,
	opts TrainOptions,
) (*Model, error) {
	if len(samples) == 0 {
		return nil, errors.New("empty training dataset")
	}
	if len(samples[0]) == 0 {
		return nil, errors.New("empty feature vectors")
	}
	if opts.NumTrees <= 0 {
		opts.NumTrees = DefaultTrainOptions().NumTrees
	}
	if opts.SampleSize <= 0 {
		opts.SampleSize = DefaultTrainOptions().SampleSize
	}

	featureCount := len(samples[0])
	if len(featureNames) != featureCount {
		featureNames = make([]string, featureCount)
		for i := range featureNames {
			featureNames[i] = "f"
		}
	}

	means, stds := fitNormalizer(samples)
	normalized := normalizeBatch(samples, means, stds)

	options := goiforest.Options{
		DetectionType: goiforest.DetectionTypeThreshold,
		Threshold:     0.6,
		NumTrees:      opts.NumTrees,
		SampleSize:    opts.SampleSize,
	}
	forest := goiforest.NewWithOptions(options)
	forest.Fit(normalized)

	a := Artifact{
		ModelKey:     modelKey,
		Interval:     interval,
		FeatureNames: append([]string(nil), featureNames...),
		Means:        means,
		Stds:         stds,
		Options:      *forest.Options,
		Trees:        forest.Trees,
		TrainedFrom:  trainedFrom.UTC(),
		TrainedTo:    trainedTo.UTC(),
	}
	return &Model{artifact: a, forest: forest}, nil
}

func (m *Model) PredictScore(sample []float64) float64 {
	if m == nil || m.forest == nil || len(sample) != len(m.artifact.Means) {
		return 0
	}
	normalized := normalize(sample, m.artifact.Means, m.artifact.Stds)
	scores := m.forest.Score([][]float64{normalized})
	if len(scores) == 0 {
		return 0
	}
	score := scores[0]
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func (m *Model) PredictBatch(samples [][]float64) []float64 {
	out := make([]float64, len(samples))
	for i := range samples {
		out[i] = m.PredictScore(samples[i])
	}
	return out
}

func (m *Model) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, errors.New("nil model")
	}
	return json.Marshal(m.artifact)
}

func UnmarshalBinary(blob []byte) (*Model, error) {
	if len(blob) == 0 {
		return nil, errors.New("empty artifact")
	}
	var a Artifact
	if err := json.Unmarshal(blob, &a); err != nil {
		return nil, err
	}
	if len(a.Means) == 0 || len(a.Means) != len(a.Stds) || len(a.Trees) == 0 {
		return nil, errors.New("invalid artifact")
	}
	forest := goiforest.NewWithOptions(a.Options)
	forest.Trees = a.Trees
	return &Model{artifact: a, forest: forest}, nil
}

func (m *Model) FeatureNames() []string {
	if m == nil {
		return nil
	}
	out := make([]string, len(m.artifact.FeatureNames))
	copy(out, m.artifact.FeatureNames)
	return out
}

func fitNormalizer(samples [][]float64) ([]float64, []float64) {
	featureCount := len(samples[0])
	means := make([]float64, featureCount)
	stds := make([]float64, featureCount)
	for j := 0; j < featureCount; j++ {
		for i := range samples {
			means[j] += samples[i][j]
		}
		means[j] /= float64(len(samples))
		for i := range samples {
			d := samples[i][j] - means[j]
			stds[j] += d * d
		}
		stds[j] = math.Sqrt(stds[j] / float64(len(samples)))
		if stds[j] == 0 {
			stds[j] = 1
		}
	}
	return means, stds
}

func normalizeBatch(samples [][]float64, means, stds []float64) [][]float64 {
	out := make([][]float64, len(samples))
	for i := range samples {
		out[i] = normalize(samples[i], means, stds)
	}
	return out
}

func normalize(in, means, stds []float64) []float64 {
	out := make([]float64, len(in))
	for i := range in {
		out[i] = (in[i] - means[i]) / stds[i]
	}
	return out
}
