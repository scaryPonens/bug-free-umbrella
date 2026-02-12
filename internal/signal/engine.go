package signal

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"
)

const (
	rsiPeriod        = 14
	macdFastPeriod   = 12
	macdSlowPeriod   = 26
	macdSignalPeriod = 9
	bollingerPeriod  = 20
	bollingerStdDevs = 2.0
	volumeWindow     = 20
	volumeZThreshold = 2.0
	squeezeThreshold = 0.08
)

type Engine struct {
	now func() time.Time
}

type event struct {
	direction domain.SignalDirection
	details   string
}

func NewEngine(now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{now: now}
}

// Generate produces deterministic signals using the most recent completed candle.
func (e *Engine) Generate(candles []*domain.Candle) []domain.Signal {
	normalized := normalizeCandles(candles)
	if len(normalized) < 2 {
		return nil
	}

	latest := normalized[len(normalized)-1]
	result := make([]domain.Signal, 0, 4)

	if ev, ok := detectRSI(normalized); ok {
		result = append(result, e.newSignal(latest, domain.IndicatorRSI, ev))
	}
	if ev, ok := detectMACD(normalized); ok {
		result = append(result, e.newSignal(latest, domain.IndicatorMACD, ev))
	}
	if ev, ok := detectBollinger(normalized); ok {
		result = append(result, e.newSignal(latest, domain.IndicatorBollinger, ev))
	}
	if ev, ok := detectVolumeAnomaly(normalized); ok {
		result = append(result, e.newSignal(latest, domain.IndicatorVolumeZ, ev))
	}

	return result
}

func (e *Engine) newSignal(candle domain.Candle, indicator string, ev event) domain.Signal {
	ts := candle.OpenTime.UTC()
	if ts.IsZero() {
		ts = e.now().UTC()
	}

	return domain.Signal{
		Symbol:    strings.ToUpper(candle.Symbol),
		Interval:  candle.Interval,
		Indicator: indicator,
		Timestamp: ts,
		Risk:      riskFor(indicator, candle.Interval),
		Direction: ev.direction,
		Details:   ev.details,
	}
}

func normalizeCandles(in []*domain.Candle) []domain.Candle {
	out := make([]domain.Candle, 0, len(in))
	for _, c := range in {
		if c == nil {
			continue
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OpenTime.Before(out[j].OpenTime)
	})
	return out
}

func detectRSI(candles []domain.Candle) (event, bool) {
	closes := extractCloses(candles)
	series := rsiSeries(closes, rsiPeriod)
	if len(series) < 2 {
		return event{}, false
	}
	prev := series[len(series)-2]
	curr := series[len(series)-1]
	if math.IsNaN(prev) || math.IsNaN(curr) {
		return event{}, false
	}

	if prev >= 30 && curr < 30 {
		return event{direction: domain.DirectionLong, details: fmt.Sprintf("rsi %.2f crossed below 30", curr)}, true
	}
	if prev <= 70 && curr > 70 {
		return event{direction: domain.DirectionShort, details: fmt.Sprintf("rsi %.2f crossed above 70", curr)}, true
	}
	return event{}, false
}

func detectMACD(candles []domain.Candle) (event, bool) {
	closes := extractCloses(candles)
	if len(closes) < macdSlowPeriod+macdSignalPeriod {
		return event{}, false
	}
	macdLine, signalLine := macdSeries(closes, macdFastPeriod, macdSlowPeriod, macdSignalPeriod)
	if len(macdLine) < 2 || len(signalLine) < 2 {
		return event{}, false
	}

	prevDelta := macdLine[len(macdLine)-2] - signalLine[len(signalLine)-2]
	currDelta := macdLine[len(macdLine)-1] - signalLine[len(signalLine)-1]

	if prevDelta <= 0 && currDelta > 0 {
		return event{direction: domain.DirectionLong, details: fmt.Sprintf("macd bullish crossover (%.4f)", currDelta)}, true
	}
	if prevDelta >= 0 && currDelta < 0 {
		return event{direction: domain.DirectionShort, details: fmt.Sprintf("macd bearish crossover (%.4f)", currDelta)}, true
	}
	return event{}, false
}

func detectBollinger(candles []domain.Candle) (event, bool) {
	closes := extractCloses(candles)
	if len(closes) < bollingerPeriod+1 {
		return event{}, false
	}

	prevIdx := len(closes) - 2
	currIdx := len(closes) - 1

	prevMean, prevStd := meanStd(closes[prevIdx-bollingerPeriod+1 : prevIdx+1])
	currMean, currStd := meanStd(closes[currIdx-bollingerPeriod+1 : currIdx+1])
	if prevMean == 0 || currMean == 0 {
		return event{}, false
	}

	prevUpper := prevMean + bollingerStdDevs*prevStd
	prevLower := prevMean - bollingerStdDevs*prevStd
	currUpper := currMean + bollingerStdDevs*currStd
	currLower := currMean - bollingerStdDevs*currStd
	prevWidth := (prevUpper - prevLower) / prevMean

	if prevWidth > squeezeThreshold {
		return event{}, false
	}

	prevClose := closes[prevIdx]
	currClose := closes[currIdx]

	if prevClose <= prevUpper && currClose > currUpper {
		return event{direction: domain.DirectionLong, details: fmt.Sprintf("bollinger squeeze breakout above upper band (width %.3f)", prevWidth)}, true
	}
	if prevClose >= prevLower && currClose < currLower {
		return event{direction: domain.DirectionShort, details: fmt.Sprintf("bollinger squeeze breakdown below lower band (width %.3f)", prevWidth)}, true
	}
	return event{}, false
}

func detectVolumeAnomaly(candles []domain.Candle) (event, bool) {
	if len(candles) < volumeWindow+1 {
		return event{}, false
	}
	volumes := extractVolumes(candles)
	window := volumes[len(volumes)-1-volumeWindow : len(volumes)-1]
	mean, std := meanStd(window)
	if std == 0 {
		return event{}, false
	}

	currVolume := volumes[len(volumes)-1]
	z := (currVolume - mean) / std
	if z < volumeZThreshold {
		return event{}, false
	}

	closes := extractCloses(candles)
	prevClose := closes[len(closes)-2]
	currClose := closes[len(closes)-1]
	direction := domain.DirectionHold
	if currClose > prevClose {
		direction = domain.DirectionLong
	} else if currClose < prevClose {
		direction = domain.DirectionShort
	}

	return event{direction: direction, details: fmt.Sprintf("volume z-score %.2f", z)}, true
}

func extractCloses(candles []domain.Candle) []float64 {
	values := make([]float64, len(candles))
	for i := range candles {
		values[i] = candles[i].Close
	}
	return values
}

func extractVolumes(candles []domain.Candle) []float64 {
	values := make([]float64, len(candles))
	for i := range candles {
		values[i] = candles[i].Volume
	}
	return values
}

func rsiSeries(closes []float64, period int) []float64 {
	if len(closes) <= period {
		return nil
	}
	series := make([]float64, len(closes))
	for i := range series {
		series[i] = math.NaN()
	}

	var gainSum float64
	var lossSum float64
	for i := 1; i <= period; i++ {
		delta := closes[i] - closes[i-1]
		if delta > 0 {
			gainSum += delta
		} else {
			lossSum -= delta
		}
	}
	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)
	series[period] = rsiFromAvg(avgGain, avgLoss)

	for i := period + 1; i < len(closes); i++ {
		delta := closes[i] - closes[i-1]
		gain := math.Max(delta, 0)
		loss := math.Max(-delta, 0)
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		series[i] = rsiFromAvg(avgGain, avgLoss)
	}

	return series
}

func rsiFromAvg(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func macdSeries(values []float64, fast, slow, signal int) ([]float64, []float64) {
	fastEMA := emaSeries(values, fast)
	slowEMA := emaSeries(values, slow)
	macdLine := make([]float64, len(values))
	for i := range values {
		macdLine[i] = fastEMA[i] - slowEMA[i]
	}
	signalLine := emaSeries(macdLine, signal)
	return macdLine, signalLine
}

func emaSeries(values []float64, period int) []float64 {
	if len(values) == 0 {
		return nil
	}
	alpha := 2.0 / (float64(period) + 1.0)
	out := make([]float64, len(values))
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

func meanStd(values []float64) (mean, std float64) {
	if len(values) == 0 {
		return 0, 0
	}
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	if len(values) == 1 {
		return mean, 0
	}
	for _, v := range values {
		d := v - mean
		std += d * d
	}
	std = math.Sqrt(std / float64(len(values)))
	return mean, std
}

func riskFor(indicator, interval string) domain.RiskLevel {
	switch indicator {
	case domain.IndicatorRSI:
		switch interval {
		case "1d", "4h":
			return domain.RiskLevel2
		case "1h":
			return domain.RiskLevel3
		case "15m", "5m":
			return domain.RiskLevel4
		}
	case domain.IndicatorMACD:
		switch interval {
		case "5m":
			return domain.RiskLevel5
		case "15m":
			return domain.RiskLevel4
		default:
			return domain.RiskLevel3
		}
	case domain.IndicatorBollinger:
		switch interval {
		case "5m":
			return domain.RiskLevel5
		case "15m", "1h":
			return domain.RiskLevel4
		default:
			return domain.RiskLevel3
		}
	case domain.IndicatorVolumeZ:
		switch interval {
		case "5m", "15m":
			return domain.RiskLevel4
		default:
			return domain.RiskLevel3
		}
	}
	return domain.RiskLevel3
}
