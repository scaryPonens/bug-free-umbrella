package signal

import (
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"
)

func TestGenerateVolumeAnomalySignal(t *testing.T) {
	engine := NewEngine(func() time.Time { return time.Unix(0, 0).UTC() })

	candles := make([]*domain.Candle, 0, 30)
	base := time.Unix(0, 0).UTC()
	for i := 0; i < 25; i++ {
		vol := 100.0 + float64(i%5)
		if i == 24 {
			vol = 1000
		}
		candles = append(candles, &domain.Candle{
			Symbol:   "BTC",
			Interval: "15m",
			OpenTime: base.Add(time.Duration(i) * time.Minute),
			Close:    100 + float64(i),
			Volume:   vol,
		})
	}

	signals := engine.Generate(candles)
	if len(signals) == 0 {
		t.Fatal("expected at least one signal")
	}

	found := false
	for _, s := range signals {
		if s.Indicator == domain.IndicatorVolumeZ {
			found = true
			if s.Direction != domain.DirectionLong {
				t.Fatalf("expected long direction, got %s", s.Direction)
			}
			if s.Risk != domain.RiskLevel4 {
				t.Fatalf("expected risk 4 for 15m volume anomaly, got %d", s.Risk)
			}
		}
	}
	if !found {
		t.Fatal("expected volume anomaly signal")
	}
}

func TestDetectBollingerBreakout(t *testing.T) {
	candles := make([]domain.Candle, 0, 30)
	base := time.Unix(0, 0).UTC()
	for i := 0; i < 20; i++ {
		closeVal := 100.0
		if i%2 == 0 {
			closeVal = 100.1
		}
		candles = append(candles, domain.Candle{
			Symbol:   "ETH",
			Interval: "5m",
			OpenTime: base.Add(time.Duration(i) * time.Minute),
			Close:    closeVal,
			Volume:   100,
		})
	}
	candles = append(candles, domain.Candle{
		Symbol:   "ETH",
		Interval: "5m",
		OpenTime: base.Add(20 * time.Minute),
		Close:    110,
		Volume:   110,
	})

	ev, ok := detectBollinger(candles)
	if !ok {
		t.Fatal("expected bollinger signal")
	}
	if ev.direction != domain.DirectionLong {
		t.Fatalf("expected long direction, got %s", ev.direction)
	}
}

func TestRiskForMapping(t *testing.T) {
	if got := riskFor(domain.IndicatorRSI, "1d"); got != domain.RiskLevel2 {
		t.Fatalf("expected RSI 1d risk=2, got %d", got)
	}
	if got := riskFor(domain.IndicatorMACD, "15m"); got != domain.RiskLevel4 {
		t.Fatalf("expected MACD 15m risk=4, got %d", got)
	}
	if got := riskFor(domain.IndicatorBollinger, "5m"); got != domain.RiskLevel5 {
		t.Fatalf("expected Bollinger 5m risk=5, got %d", got)
	}
}

func TestGenerateReturnsNilForInsufficientCandles(t *testing.T) {
	engine := NewEngine(nil)
	candles := []*domain.Candle{{Symbol: "BTC", Interval: "1h", OpenTime: time.Now().UTC(), Close: 1, Volume: 1}}
	if got := engine.Generate(candles); len(got) != 0 {
		t.Fatalf("expected no signals, got %d", len(got))
	}
}
