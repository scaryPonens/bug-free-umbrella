package tui

import (
	"testing"
	"time"

	"bug-free-umbrella/internal/repository"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBacktestModelInitialState(t *testing.T) {
	m := NewBacktestModel(testServices())
	if m.ActiveView() != backtestViewAccuracy {
		t.Fatalf("expected accuracy view, got %d", m.ActiveView())
	}
	if m.HasData() {
		t.Fatal("expected no data initially")
	}
}

func TestBacktestModelToggleView(t *testing.T) {
	m := NewBacktestModel(testServices())
	m.SetSize(120, 40)

	// Press 'v' to toggle
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if updated.ActiveView() != backtestViewPredictions {
		t.Fatalf("expected predictions view after toggle, got %d", updated.ActiveView())
	}

	// Toggle back
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if updated.ActiveView() != backtestViewAccuracy {
		t.Fatalf("expected accuracy view after second toggle, got %d", updated.ActiveView())
	}
}

func TestBacktestModelUpdateSummary(t *testing.T) {
	m := NewBacktestModel(testServices())
	m.SetSize(120, 40)

	summary := []repository.DailyAccuracy{
		{ModelKey: "ml_logreg_up4h", Total: 100, Correct: 78, Accuracy: 0.78},
	}

	updated, _ := m.Update(backtestSummaryMsg(summary))
	if !updated.HasData() {
		t.Fatal("expected data after summary update")
	}
}

func TestBacktestModelUpdateDaily(t *testing.T) {
	m := NewBacktestModel(testServices())
	m.SetSize(120, 40)

	daily := []repository.DailyAccuracy{
		{ModelKey: "ml_logreg_up4h", DayUTC: time.Now(), Total: 12, Correct: 9, Accuracy: 0.75},
	}

	updated, _ := m.Update(backtestDailyMsg(daily))
	if !updated.HasData() {
		t.Fatal("expected data after daily update")
	}
}

func TestBacktestModelViewEmpty(t *testing.T) {
	m := NewBacktestModel(testServices())
	m.SetSize(120, 40)
	m.loading = false

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestBacktestModelViewWithData(t *testing.T) {
	m := NewBacktestModel(testServices())
	m.SetSize(120, 40)
	m.loading = false
	m.summary = []repository.DailyAccuracy{
		{ModelKey: "ml_logreg_up4h", Total: 100, Correct: 78, Accuracy: 0.78},
	}

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view with data")
	}
}
