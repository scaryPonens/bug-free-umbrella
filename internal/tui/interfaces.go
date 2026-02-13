package tui

import (
	"context"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"
)

// PriceQuerier provides price data to the TUI.
type PriceQuerier interface {
	GetCurrentPrices(ctx context.Context) ([]*domain.PriceSnapshot, error)
}

// SignalQuerier provides signal data to the TUI.
type SignalQuerier interface {
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

// AdvisorQuerier provides LLM advisor access to the TUI.
type AdvisorQuerier interface {
	Ask(ctx context.Context, chatID int64, message string) (string, error)
}

// BacktestQuerier provides ML backtest data to the TUI.
type BacktestQuerier interface {
	GetDailyAccuracy(ctx context.Context, modelKey string, days int) ([]repository.DailyAccuracy, error)
	GetAccuracySummary(ctx context.Context) ([]repository.DailyAccuracy, error)
	ListRecentPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error)
}

// SSHChatIDOffset is the base offset for generating synthetic chat IDs
// for SSH users. The final chat ID is SSHChatIDOffset - user.ID.
// This avoids collisions with Telegram chat IDs.
const SSHChatIDOffset int64 = -1_000_000

// Services bundles all service dependencies injected into the TUI.
type Services struct {
	Prices   PriceQuerier
	Signals  SignalQuerier
	Advisor  AdvisorQuerier
	Backtest BacktestQuerier
	UserID   int64
	Username string
}

// ChatID returns the synthetic chat ID for this SSH session.
func (s Services) ChatID() int64 {
	return SSHChatIDOffset - s.UserID
}
