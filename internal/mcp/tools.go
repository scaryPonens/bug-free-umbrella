package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTools(server *mcp.Server, prices PriceReader, signals SignalReaderWriter) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "prices_list_latest",
		Description: "Get latest market snapshots for all supported symbols",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ pricesListLatestInput) (*mcp.CallToolResult, pricesListLatestOutput, error) {
		if prices == nil {
			return nil, pricesListLatestOutput{}, fmt.Errorf("price service unavailable")
		}
		result, err := prices.GetCurrentPrices(ctx)
		if err != nil {
			return nil, pricesListLatestOutput{}, err
		}
		return nil, pricesListLatestOutput{Prices: result}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "prices_get_by_symbol",
		Description: "Get latest market snapshot for one symbol",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pricesGetBySymbolInput) (*mcp.CallToolResult, pricesGetBySymbolOutput, error) {
		if prices == nil {
			return nil, pricesGetBySymbolOutput{}, fmt.Errorf("price service unavailable")
		}
		symbol, err := normalizeSymbol(in.Symbol)
		if err != nil {
			return nil, pricesGetBySymbolOutput{}, err
		}
		result, err := prices.GetCurrentPrice(ctx, symbol)
		if err != nil {
			return nil, pricesGetBySymbolOutput{}, err
		}
		return nil, pricesGetBySymbolOutput{Price: result}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "candles_list",
		Description: "Get OHLCV candles by symbol, interval, and limit",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in candlesListInput) (*mcp.CallToolResult, candlesListOutput, error) {
		if prices == nil {
			return nil, candlesListOutput{}, fmt.Errorf("price service unavailable")
		}
		symbol, err := normalizeSymbol(in.Symbol)
		if err != nil {
			return nil, candlesListOutput{}, err
		}
		interval, err := normalizeInterval(in.Interval)
		if err != nil {
			return nil, candlesListOutput{}, err
		}
		limit := normalizeCandleLimit(in.Limit)

		result, err := prices.GetCandles(ctx, symbol, interval, limit)
		if err != nil {
			return nil, candlesListOutput{}, err
		}
		return nil, candlesListOutput{Symbol: symbol, Interval: interval, Candles: result}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "signals_list",
		Description: "Get recent generated trading signals with optional filters",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in signalsListInput) (*mcp.CallToolResult, signalsListOutput, error) {
		if signals == nil {
			return nil, signalsListOutput{}, fmt.Errorf("signal service unavailable")
		}
		filter, err := normalizeSignalFilter(in)
		if err != nil {
			return nil, signalsListOutput{}, err
		}
		result, err := signals.ListSignals(ctx, filter)
		if err != nil {
			return nil, signalsListOutput{}, err
		}
		return nil, signalsListOutput{Signals: result}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "signals_generate",
		Description: "Generate and persist deterministic technical signals for a symbol",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in signalsGenerateInput) (*mcp.CallToolResult, signalsGenerateOutput, error) {
		if signals == nil {
			return nil, signalsGenerateOutput{}, fmt.Errorf("signal service unavailable")
		}
		symbol, err := normalizeSymbol(in.Symbol)
		if err != nil {
			return nil, signalsGenerateOutput{}, err
		}
		intervals, err := normalizeGenerateIntervals(in.Intervals)
		if err != nil {
			return nil, signalsGenerateOutput{}, err
		}

		generated, err := signals.GenerateForSymbol(ctx, symbol, intervals)
		if err != nil {
			return nil, signalsGenerateOutput{}, err
		}
		return nil, signalsGenerateOutput{GeneratedCount: len(generated), Signals: generated}, nil
	})
}
