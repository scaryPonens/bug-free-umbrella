package chart

import (
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
)

func TestRenderSignalChartByIndicator(t *testing.T) {
	renderer := NewRenderer()
	candles := buildTestCandles(160)
	indicators := []string{
		domain.IndicatorRSI,
		domain.IndicatorMACD,
		domain.IndicatorBollinger,
		domain.IndicatorVolumeZ,
	}

	for _, indicator := range indicators {
		t.Run(indicator, func(t *testing.T) {
			image, err := renderer.RenderSignalChart(candles, domain.Signal{
				Symbol:    "BTC",
				Interval:  "1h",
				Indicator: indicator,
				Direction: domain.DirectionLong,
				Timestamp: time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if image == nil || len(image.Bytes) == 0 {
				t.Fatal("expected non-empty image bytes")
			}
			if image.Ref.MimeType != "image/png" {
				t.Fatalf("expected image/png mime type, got %s", image.Ref.MimeType)
			}
		})
	}
}

func buildTestCandles(count int) []*domain.Candle {
	base := time.Now().UTC().Add(-time.Duration(count) * time.Hour)
	out := make([]*domain.Candle, 0, count)
	price := 50000.0
	for i := 0; i < count; i++ {
		step := float64((i%9)-4) * 18
		open := price
		close := price + step
		high := maxFloat(open, close) + 22
		low := minFloat(open, close) - 20
		volume := 1000 + float64((i%17)*80)
		if i%25 == 0 {
			volume *= 2.4
		}
		out = append(out, &domain.Candle{
			Symbol:   "BTC",
			Interval: "1h",
			OpenTime: base.Add(time.Duration(i) * time.Hour),
			Open:     open,
			High:     high,
			Low:      low,
			Close:    close,
			Volume:   volume,
		})
		price = close
	}
	return out
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
