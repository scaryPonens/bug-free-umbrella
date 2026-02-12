package mcp

import (
	"context"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsListAndInvoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv, _, signals := testServer()
	session, shutdown, err := connectInMemory(ctx, srv)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer shutdown()
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools.Tools) < 5 {
		t.Fatalf("expected at least 5 tools, got %d", len(tools.Tools))
	}

	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "prices_get_by_symbol", Arguments: map[string]any{"symbol": "btc"}})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res.Content)
	}

	res, err = session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "signals_generate", Arguments: map[string]any{"symbol": "BTC", "intervals": []string{"1h"}}})
	if err != nil {
		t.Fatalf("generate tool failed: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected generate tool error: %+v", res.Content)
	}
	if signals.lastGenerateSymbol != "BTC" {
		t.Fatalf("expected generate symbol BTC, got %s", signals.lastGenerateSymbol)
	}
	if len(signals.lastGenerateIntervals) != 1 || signals.lastGenerateIntervals[0] != "1h" {
		t.Fatalf("unexpected generate intervals: %+v", signals.lastGenerateIntervals)
	}
}

func TestToolsValidationFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv, _, _ := testServer()
	session, shutdown, err := connectInMemory(ctx, srv)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer shutdown()
	defer session.Close()

	res, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "candles_list",
		Arguments: map[string]any{"symbol": "FAKE", "interval": "1h"},
	})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected tool-level validation error")
	}
}
