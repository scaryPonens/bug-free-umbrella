package tui

import (
	"testing"

	"bug-free-umbrella/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSignalExplorerFilterCycling(t *testing.T) {
	m := NewSignalExplorerModel(testServices())
	m.SetSize(120, 40)

	// Initial state: all filters at index 0 (ALL)
	si, ri, ii := m.FilterState()
	if si != 0 || ri != 0 || ii != 0 {
		t.Fatalf("expected all filters at 0, got %d/%d/%d", si, ri, ii)
	}

	// Press 's' to cycle symbol filter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	si, _, _ = updated.FilterState()
	if si != 1 {
		t.Fatalf("expected symbol index 1 after pressing s, got %d", si)
	}

	// Press 'r' to cycle risk filter
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	_, ri, _ = updated.FilterState()
	if ri != 1 {
		t.Fatalf("expected risk index 1 after pressing r, got %d", ri)
	}

	// Press 'i' to cycle indicator filter
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	_, _, ii = updated.FilterState()
	if ii != 1 {
		t.Fatalf("expected indicator index 1 after pressing i, got %d", ii)
	}
}

func TestSignalExplorerUpdateSignals(t *testing.T) {
	m := NewSignalExplorerModel(testServices())
	m.SetSize(120, 40)

	signals := []domain.Signal{
		{ID: 1, Symbol: "BTC", Interval: "1h", Indicator: "rsi", Direction: domain.DirectionLong, Risk: 2},
		{ID: 2, Symbol: "ETH", Interval: "4h", Indicator: "macd", Direction: domain.DirectionShort, Risk: 3},
	}

	updated, _ := m.Update(filteredSignalsMsg(signals))
	if updated.SignalCount() != 2 {
		t.Fatalf("expected 2 signals, got %d", updated.SignalCount())
	}
}

func TestSignalExplorerViewEmpty(t *testing.T) {
	m := NewSignalExplorerModel(testServices())
	m.SetSize(120, 40)
	m.loading = false

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestSignalExplorerScrolling(t *testing.T) {
	m := NewSignalExplorerModel(testServices())
	m.SetSize(120, 20)
	m.loading = false

	// Add many signals
	for i := 0; i < 50; i++ {
		m.signals = append(m.signals, domain.Signal{
			ID:        int64(i),
			Symbol:    "BTC",
			Interval:  "1h",
			Indicator: "rsi",
			Direction: domain.DirectionLong,
			Risk:      2,
		})
	}

	// Scroll down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if updated.scrollOffset != 1 {
		t.Fatalf("expected scroll offset 1, got %d", updated.scrollOffset)
	}

	// Scroll up
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if updated.scrollOffset != 0 {
		t.Fatalf("expected scroll offset 0, got %d", updated.scrollOffset)
	}
}
