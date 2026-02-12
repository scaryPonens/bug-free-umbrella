package mcp

import (
	"context"

	"bug-free-umbrella/internal/domain"
)

// PriceReader exposes read operations for market data.
type PriceReader interface {
	GetCurrentPrices(ctx context.Context) ([]*domain.PriceSnapshot, error)
	GetCurrentPrice(ctx context.Context, symbol string) (*domain.PriceSnapshot, error)
	GetCandles(ctx context.Context, symbol, interval string, limit int) ([]*domain.Candle, error)
}

// SignalReaderWriter exposes read/generate operations for signals.
type SignalReaderWriter interface {
	ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error)
	GenerateForSymbol(ctx context.Context, symbol string, intervals []string) ([]domain.Signal, error)
}
