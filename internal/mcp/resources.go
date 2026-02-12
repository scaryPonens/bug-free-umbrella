package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"bug-free-umbrella/internal/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResources(server *mcp.Server, prices PriceReader, signals SignalReaderWriter) {
	server.AddResource(&mcp.Resource{
		URI:         "market://supported-symbols",
		Name:        "supported-symbols",
		Description: "List of symbols supported by the service",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		_ = ctx
		return jsonResource(req.Params.URI, domain.SupportedSymbols)
	})

	server.AddResource(&mcp.Resource{
		URI:         "market://supported-intervals",
		Name:        "supported-intervals",
		Description: "List of candle intervals supported by the service",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		_ = ctx
		return jsonResource(req.Params.URI, domain.SupportedIntervals)
	})

	server.AddResource(&mcp.Resource{
		URI:         "prices://latest",
		Name:        "prices-latest",
		Description: "Latest price snapshots for all supported symbols",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if prices == nil {
			return nil, fmt.Errorf("price service unavailable")
		}
		list, err := prices.GetCurrentPrices(ctx)
		if err != nil {
			return nil, err
		}
		return jsonResource(req.Params.URI, pricesListLatestOutput{Prices: list})
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "prices://symbol/{symbol}",
		Name:        "price-by-symbol",
		Description: "Latest price snapshot for a specific symbol",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if prices == nil {
			return nil, fmt.Errorf("price service unavailable")
		}

		parsed, err := url.Parse(req.Params.URI)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		if parsed.Scheme != "prices" || parsed.Host != "symbol" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		symbol := strings.Trim(strings.TrimSpace(parsed.Path), "/")
		symbol, err = normalizeSymbol(symbol)
		if err != nil {
			return nil, err
		}

		snapshot, err := prices.GetCurrentPrice(ctx, symbol)
		if err != nil {
			return nil, err
		}
		return jsonResource(req.Params.URI, pricesGetBySymbolOutput{Price: snapshot})
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "candles://{symbol}/{interval}{?limit}",
		Name:        "candles-by-symbol-interval",
		Description: "OHLCV candles for a symbol and interval; optional limit query param",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if prices == nil {
			return nil, fmt.Errorf("price service unavailable")
		}

		parsed, err := url.Parse(req.Params.URI)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		if parsed.Scheme != "candles" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		symbol, err := normalizeSymbol(parsed.Host)
		if err != nil {
			return nil, err
		}
		interval, err := normalizeInterval(strings.Trim(strings.TrimSpace(parsed.Path), "/"))
		if err != nil {
			return nil, err
		}

		limit := defaultCandleLimit
		if rawLimit := strings.TrimSpace(parsed.Query().Get("limit")); rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil {
				return nil, fmt.Errorf("invalid limit: %s", rawLimit)
			}
			limit = normalizeCandleLimit(n)
		}

		candles, err := prices.GetCandles(ctx, symbol, interval, limit)
		if err != nil {
			return nil, err
		}
		return jsonResource(req.Params.URI, candlesListOutput{Symbol: symbol, Interval: interval, Candles: candles})
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "signals://latest{?symbol,risk,indicator,limit}",
		Name:        "signals-latest",
		Description: "Recent generated signals with optional symbol/risk/indicator/limit query params",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if signals == nil {
			return nil, fmt.Errorf("signal service unavailable")
		}

		parsed, err := url.Parse(req.Params.URI)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		if parsed.Scheme != "signals" || parsed.Host != "latest" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}

		input := signalsListInput{
			Symbol:    parsed.Query().Get("symbol"),
			Indicator: parsed.Query().Get("indicator"),
			Limit:     defaultSignalLimit,
		}
		if rawLimit := strings.TrimSpace(parsed.Query().Get("limit")); rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil {
				return nil, fmt.Errorf("invalid limit: %s", rawLimit)
			}
			input.Limit = n
		}
		if rawRisk := strings.TrimSpace(parsed.Query().Get("risk")); rawRisk != "" {
			n, err := strconv.Atoi(rawRisk)
			if err != nil {
				return nil, fmt.Errorf("invalid risk: %s", rawRisk)
			}
			input.Risk = &n
		}

		filter, err := normalizeSignalFilter(input)
		if err != nil {
			return nil, err
		}
		list, err := signals.ListSignals(ctx, filter)
		if err != nil {
			return nil, err
		}
		return jsonResource(req.Params.URI, signalsListOutput{Signals: list})
	})
}

func jsonResource(uri string, payload any) (*mcp.ReadResourceResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(body),
		}},
	}, nil
}
