package tui

import (
	"context"
	"testing"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"

	tea "github.com/charmbracelet/bubbletea"
)

// --- stub services ---

type stubPriceQuerier struct {
	prices []*domain.PriceSnapshot
	err    error
}

func (s *stubPriceQuerier) GetCurrentPrices(ctx context.Context) ([]*domain.PriceSnapshot, error) {
	return s.prices, s.err
}

type stubSignalQuerier struct {
	signals []domain.Signal
	err     error
}

func (s *stubSignalQuerier) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	return s.signals, s.err
}

type stubAdvisorQuerier struct {
	reply string
	err   error
}

func (s *stubAdvisorQuerier) Ask(ctx context.Context, chatID int64, message string) (string, error) {
	return s.reply, s.err
}

type stubBacktestQuerier struct {
	summary     []repository.DailyAccuracy
	daily       []repository.DailyAccuracy
	predictions []domain.MLPrediction
	err         error
}

func (s *stubBacktestQuerier) GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error) {
	return s.daily, s.err
}

func (s *stubBacktestQuerier) GetAccuracySummary(ctx context.Context) ([]repository.DailyAccuracy, error) {
	return s.summary, s.err
}

func (s *stubBacktestQuerier) ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	return s.predictions, s.err
}

func testServices() Services {
	return Services{
		Prices:   &stubPriceQuerier{},
		Signals:  &stubSignalQuerier{},
		Advisor:  &stubAdvisorQuerier{reply: "test reply"},
		Backtest: &stubBacktestQuerier{},
		UserID:   1,
		Username: "testuser",
	}
}

func TestAppModelInitialTab(t *testing.T) {
	m := NewAppModel(testServices())
	if m.ActiveTab() != TabDashboard {
		t.Fatalf("expected TabDashboard, got %d", m.ActiveTab())
	}
}

func TestAppModelTabSwitchByNumber(t *testing.T) {
	m := NewAppModel(testServices())
	m.SetSize(120, 40)

	// Press '2' to switch to chat
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	app := updated.(AppModel)
	if app.ActiveTab() != TabChat {
		t.Fatalf("expected TabChat after pressing 2, got %d", app.ActiveTab())
	}

	// Press '3' to switch to signals
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	app = updated.(AppModel)
	if app.ActiveTab() != TabSignals {
		t.Fatalf("expected TabSignals after pressing 3, got %d", app.ActiveTab())
	}

	// Press '4' to switch to backtest
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	app = updated.(AppModel)
	if app.ActiveTab() != TabBacktest {
		t.Fatalf("expected TabBacktest after pressing 4, got %d", app.ActiveTab())
	}

	// Press '1' to switch back to dashboard
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	app = updated.(AppModel)
	if app.ActiveTab() != TabDashboard {
		t.Fatalf("expected TabDashboard after pressing 1, got %d", app.ActiveTab())
	}
}

func TestAppModelTabSwitchByTab(t *testing.T) {
	m := NewAppModel(testServices())
	m.SetSize(120, 40)

	// Press Tab to go to next
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	app := updated.(AppModel)
	if app.ActiveTab() != TabChat {
		t.Fatalf("expected TabChat after Tab, got %d", app.ActiveTab())
	}

	// Press Shift+Tab to go back
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = updated.(AppModel)
	if app.ActiveTab() != TabDashboard {
		t.Fatalf("expected TabDashboard after Shift+Tab, got %d", app.ActiveTab())
	}
}

func TestAppModelWindowResize(t *testing.T) {
	m := NewAppModel(testServices())

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	app := updated.(AppModel)
	if app.width != 100 || app.height != 50 {
		t.Fatalf("expected 100x50, got %dx%d", app.width, app.height)
	}
}

func TestAppModelViewRendersWithoutPanic(t *testing.T) {
	m := NewAppModel(testServices())
	m.SetSize(120, 40)

	// Render all tabs without panicking
	for _, tab := range []Tab{TabDashboard, TabChat, TabSignals, TabBacktest} {
		m.activeTab = tab
		view := m.View()
		if view == "" {
			t.Fatalf("expected non-empty view for tab %d", tab)
		}
	}
}

func TestServicesChatID(t *testing.T) {
	svc := Services{UserID: 42}
	expected := SSHChatIDOffset - 42
	if svc.ChatID() != expected {
		t.Fatalf("expected chat ID %d, got %d", expected, svc.ChatID())
	}
}
