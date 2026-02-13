package tui

import (
	"testing"

	"bug-free-umbrella/internal/domain"
)

func TestDashboardUpdatePricesMsg(t *testing.T) {
	m := NewDashboardModel(testServices())
	m.SetSize(120, 40)

	prices := []*domain.PriceSnapshot{
		{Symbol: "BTC", PriceUSD: 98000, Change24hPct: 2.3, Volume24h: 28e9},
		{Symbol: "ETH", PriceUSD: 3456, Change24hPct: -1.2, Volume24h: 15e9},
	}

	updated, _ := m.Update(pricesMsg(prices))
	if len(updated.Prices()) != 2 {
		t.Fatalf("expected 2 prices, got %d", len(updated.Prices()))
	}
	if updated.Prices()[0].Symbol != "BTC" {
		t.Fatalf("expected BTC, got %s", updated.Prices()[0].Symbol)
	}
}

func TestDashboardUpdateSignalsMsg(t *testing.T) {
	m := NewDashboardModel(testServices())
	m.SetSize(120, 40)

	signals := []domain.Signal{
		{ID: 1, Symbol: "BTC", Interval: "1h", Indicator: "rsi", Direction: domain.DirectionLong, Risk: 2},
	}

	updated, _ := m.Update(signalsMsg(signals))
	if len(updated.Signals()) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(updated.Signals()))
	}
}

func TestDashboardViewEmpty(t *testing.T) {
	m := NewDashboardModel(testServices())
	m.SetSize(120, 40)
	m.loading = false

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestDashboardViewWithData(t *testing.T) {
	m := NewDashboardModel(testServices())
	m.SetSize(120, 40)

	m.prices = []*domain.PriceSnapshot{
		{Symbol: "BTC", PriceUSD: 98000, Change24hPct: 2.3, Volume24h: 28e9},
	}
	m.signals = []domain.Signal{
		{ID: 1, Symbol: "BTC", Interval: "1h", Indicator: "rsi", Direction: domain.DirectionLong, Risk: 2},
	}
	m.loading = false

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view with data")
	}
}
