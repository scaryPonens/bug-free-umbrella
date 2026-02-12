package chart

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"

	"bug-free-umbrella/internal/domain"
)

const (
	defaultChartWidth  = 960
	defaultChartHeight = 640
	maxChartCandles    = 120
)

var (
	colBackground = color.RGBA{R: 250, G: 252, B: 255, A: 255}
	colGrid       = color.RGBA{R: 225, G: 232, B: 240, A: 255}
	colBull       = color.RGBA{R: 18, G: 140, B: 126, A: 255}
	colBear       = color.RGBA{R: 210, G: 61, B: 87, A: 255}
	colWick       = color.RGBA{R: 58, G: 64, B: 90, A: 255}
	colMarker     = color.RGBA{R: 62, G: 106, B: 214, A: 255}
	colLineA      = color.RGBA{R: 62, G: 106, B: 214, A: 255}
	colLineB      = color.RGBA{R: 255, G: 149, B: 0, A: 255}
	colBand       = color.RGBA{R: 104, G: 122, B: 146, A: 255}
	colVolume     = color.RGBA{R: 120, G: 139, B: 164, A: 255}
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) RenderSignalChart(candles []*domain.Candle, signal domain.Signal) (*domain.SignalImageData, error) {
	series := normalizeCandles(candles)
	if len(series) < 2 {
		return nil, fmt.Errorf("need at least 2 candles to render chart")
	}
	if len(series) > maxChartCandles {
		series = series[len(series)-maxChartCandles:]
	}

	img := image.NewRGBA(image.Rect(0, 0, defaultChartWidth, defaultChartHeight))
	fillRect(img, img.Bounds(), colBackground)

	mainRect := image.Rect(60, 20, defaultChartWidth-20, (defaultChartHeight*72)/100)
	auxRect := image.Rect(60, mainRect.Max.Y+16, defaultChartWidth-20, defaultChartHeight-30)
	drawGrid(img, mainRect, 8, 6)
	drawGrid(img, auxRect, 8, 3)

	if err := drawCandles(img, mainRect, series); err != nil {
		return nil, err
	}

	markerX := mapIndexToX(len(series)-1, len(series), mainRect)
	drawLine(img, markerX, mainRect.Min.Y, markerX, mainRect.Max.Y, colMarker)

	switch signal.Indicator {
	case domain.IndicatorRSI:
		drawRSI(img, auxRect, series)
	case domain.IndicatorMACD:
		drawMACD(img, auxRect, series)
	case domain.IndicatorBollinger:
		drawBollinger(img, mainRect, series)
		drawPriceDeltaBars(img, auxRect, series)
	case domain.IndicatorVolumeZ:
		drawVolumeZ(img, auxRect, series)
	default:
		return nil, fmt.Errorf("unsupported indicator: %s", signal.Indicator)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	return &domain.SignalImageData{
		Ref: domain.SignalImageRef{
			MimeType: "image/png",
			Width:    defaultChartWidth,
			Height:   defaultChartHeight,
		},
		Bytes: buf.Bytes(),
	}, nil
}

func normalizeCandles(in []*domain.Candle) []domain.Candle {
	out := make([]domain.Candle, 0, len(in))
	for _, c := range in {
		if c == nil {
			continue
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenTime.Before(out[j].OpenTime) })
	return out
}

func drawCandles(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) error {
	if len(candles) == 0 {
		return fmt.Errorf("no candles")
	}

	minPrice := candles[0].Low
	maxPrice := candles[0].High
	for _, c := range candles {
		if c.Low < minPrice {
			minPrice = c.Low
		}
		if c.High > maxPrice {
			maxPrice = c.High
		}
	}
	if maxPrice <= minPrice {
		maxPrice = minPrice + 1
	}

	candleWidth := max(3, (rect.Dx()-10)/len(candles)-1)
	for i, c := range candles {
		x := mapIndexToX(i, len(candles), rect)
		highY := mapValueToY(c.High, minPrice, maxPrice, rect)
		lowY := mapValueToY(c.Low, minPrice, maxPrice, rect)
		drawLine(img, x, highY, x, lowY, colWick)

		openY := mapValueToY(c.Open, minPrice, maxPrice, rect)
		closeY := mapValueToY(c.Close, minPrice, maxPrice, rect)
		top := min(openY, closeY)
		bottom := max(openY, closeY)
		if bottom-top < 2 {
			bottom = top + 2
		}

		bodyRect := image.Rect(x-candleWidth/2, top, x+candleWidth/2+1, bottom+1)
		bodyColor := colBull
		if c.Close < c.Open {
			bodyColor = colBear
		}
		fillRect(img, bodyRect, bodyColor)
	}
	return nil
}

func drawRSI(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) {
	closes := extractCloses(candles)
	rsi := rsiSeries(closes, 14)
	drawHorizontalValueLine(img, rect, 30, 0, 100, colBand)
	drawHorizontalValueLine(img, rect, 70, 0, 100, colBand)
	drawSeries(img, rect, rsi, 0, 100, colLineA)
}

func drawMACD(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) {
	closes := extractCloses(candles)
	macd, signal := macdSeries(closes, 12, 26, 9)
	minV, maxV := finiteBounds(macd)
	minS, maxS := finiteBounds(signal)
	minV = math.Min(minV, minS)
	maxV = math.Max(maxV, maxS)
	if minV == maxV {
		maxV = minV + 1
	}
	drawHorizontalValueLine(img, rect, 0, minV, maxV, colBand)
	drawSeries(img, rect, macd, minV, maxV, colLineA)
	drawSeries(img, rect, signal, minV, maxV, colLineB)
}

func drawBollinger(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) {
	if len(candles) < 20 {
		return
	}
	closes := extractCloses(candles)
	upper := make([]float64, len(closes))
	lower := make([]float64, len(closes))
	mean := make([]float64, len(closes))
	for i := range closes {
		upper[i] = math.NaN()
		lower[i] = math.NaN()
		mean[i] = math.NaN()
		if i < 19 {
			continue
		}
		m, s := meanStd(closes[i-19 : i+1])
		mean[i] = m
		upper[i] = m + 2*s
		lower[i] = m - 2*s
	}
	minV, maxV := finiteBounds(extractCloses(candles))
	minL, _ := finiteBounds(lower)
	_, maxU := finiteBounds(upper)
	minV = math.Min(minV, minL)
	maxV = math.Max(maxV, maxU)
	drawSeries(img, rect, upper, minV, maxV, colBand)
	drawSeries(img, rect, mean, minV, maxV, colLineB)
	drawSeries(img, rect, lower, minV, maxV, colBand)
}

func drawVolumeZ(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) {
	if len(candles) < 21 {
		return
	}
	volumes := extractVolumes(candles)
	zscores := make([]float64, len(volumes))
	for i := range zscores {
		zscores[i] = math.NaN()
		if i < 20 {
			continue
		}
		m, s := meanStd(volumes[i-20 : i])
		if s == 0 {
			continue
		}
		zscores[i] = (volumes[i] - m) / s
	}
	minV, maxV := finiteBounds(zscores)
	if minV > 0 {
		minV = 0
	}
	if maxV < 2 {
		maxV = 2
	}
	drawHorizontalValueLine(img, rect, 2.0, minV, maxV, colBand)
	drawBars(img, rect, zscores, minV, maxV, colVolume)
}

func drawPriceDeltaBars(img *image.RGBA, rect image.Rectangle, candles []domain.Candle) {
	if len(candles) < 2 {
		return
	}
	vals := make([]float64, len(candles))
	vals[0] = math.NaN()
	for i := 1; i < len(candles); i++ {
		vals[i] = candles[i].Close - candles[i-1].Close
	}
	minV, maxV := finiteBounds(vals)
	if minV == maxV {
		maxV = minV + 1
	}
	drawHorizontalValueLine(img, rect, 0, minV, maxV, colBand)
	drawBars(img, rect, vals, minV, maxV, colVolume)
}

func drawSeries(img *image.RGBA, rect image.Rectangle, series []float64, minV, maxV float64, col color.RGBA) {
	lastX, lastY := -1, -1
	for i, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			lastX, lastY = -1, -1
			continue
		}
		x := mapIndexToX(i, len(series), rect)
		y := mapValueToY(v, minV, maxV, rect)
		if lastX >= 0 {
			drawLine(img, lastX, lastY, x, y, col)
		}
		lastX, lastY = x, y
	}
}

func drawBars(img *image.RGBA, rect image.Rectangle, series []float64, minV, maxV float64, col color.RGBA) {
	barW := max(1, (rect.Dx()-10)/len(series)-1)
	zeroY := mapValueToY(0, minV, maxV, rect)
	for i, v := range series {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		x := mapIndexToX(i, len(series), rect)
		y := mapValueToY(v, minV, maxV, rect)
		top := min(y, zeroY)
		bottom := max(y, zeroY)
		fillRect(img, image.Rect(x-barW/2, top, x+barW/2+1, bottom+1), col)
	}
}

func drawGrid(img *image.RGBA, rect image.Rectangle, verticalLines, horizontalLines int) {
	for i := 0; i <= verticalLines; i++ {
		x := rect.Min.X + (rect.Dx()*i)/max(1, verticalLines)
		drawLine(img, x, rect.Min.Y, x, rect.Max.Y, colGrid)
	}
	for i := 0; i <= horizontalLines; i++ {
		y := rect.Min.Y + (rect.Dy()*i)/max(1, horizontalLines)
		drawLine(img, rect.Min.X, y, rect.Max.X, y, colGrid)
	}
}

func drawHorizontalValueLine(img *image.RGBA, rect image.Rectangle, value, minV, maxV float64, col color.RGBA) {
	y := mapValueToY(value, minV, maxV, rect)
	drawLine(img, rect.Min.X, y, rect.Max.X, y, col)
}

func mapIndexToX(idx, total int, rect image.Rectangle) int {
	if total <= 1 {
		return rect.Min.X
	}
	return rect.Min.X + (idx*(rect.Dx()-1))/(total-1)
}

func mapValueToY(value, minV, maxV float64, rect image.Rectangle) int {
	if maxV <= minV {
		return rect.Max.Y
	}
	ratio := (value - minV) / (maxV - minV)
	ratio = math.Max(0, math.Min(1, ratio))
	return rect.Max.Y - int(ratio*float64(rect.Dy()-1))
}

func finiteBounds(values []float64) (float64, float64) {
	minV := math.Inf(1)
	maxV := math.Inf(-1)
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	if math.IsInf(minV, 1) || math.IsInf(maxV, -1) {
		return 0, 1
	}
	if minV == maxV {
		return minV, maxV + 1
	}
	return minV, maxV
}

func extractCloses(candles []domain.Candle) []float64 {
	out := make([]float64, len(candles))
	for i := range candles {
		out[i] = candles[i].Close
	}
	return out
}

func extractVolumes(candles []domain.Candle) []float64 {
	out := make([]float64, len(candles))
	for i := range candles {
		out[i] = candles[i].Volume
	}
	return out
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	var mean float64
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))
	if len(values) == 1 {
		return mean, 0
	}
	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	return mean, math.Sqrt(variance)
}

func emaSeries(values []float64, period int) []float64 {
	if len(values) == 0 {
		return nil
	}
	alpha := 2.0 / (float64(period) + 1)
	out := make([]float64, len(values))
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out
}

func macdSeries(values []float64, fast, slow, signal int) ([]float64, []float64) {
	fastEMA := emaSeries(values, fast)
	slowEMA := emaSeries(values, slow)
	macd := make([]float64, len(values))
	for i := range values {
		macd[i] = fastEMA[i] - slowEMA[i]
	}
	sig := emaSeries(macd, signal)
	return macd, sig
}

func rsiSeries(closes []float64, period int) []float64 {
	if len(closes) <= period {
		return nil
	}
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
	}
	var gainSum, lossSum float64
	for i := 1; i <= period; i++ {
		d := closes[i] - closes[i-1]
		if d > 0 {
			gainSum += d
		} else {
			lossSum -= d
		}
	}
	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)
	out[period] = rsiFromAvg(avgGain, avgLoss)
	for i := period + 1; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		gain := math.Max(d, 0)
		loss := math.Max(-d, 0)
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		out[i] = rsiFromAvg(avgGain, avgLoss)
	}
	return out
}

func rsiFromAvg(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func fillRect(img *image.RGBA, rect image.Rectangle, col color.RGBA) {
	r := rect.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetRGBA(x, y, col)
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.RGBA) {
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		if image.Pt(x0, y0).In(img.Bounds()) {
			img.SetRGBA(x0, y0, col)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			if x0 == x1 {
				break
			}
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			if y0 == y1 {
				break
			}
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
