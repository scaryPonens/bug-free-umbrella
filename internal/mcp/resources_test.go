package mcp

import (
	"context"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResourcesStaticAndTemplated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv, _, signals := testServer()
	session, shutdown, err := connectInMemory(ctx, srv)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer shutdown()
	defer session.Close()

	list, err := session.ListResources(ctx, &sdkmcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("list resources failed: %v", err)
	}
	if len(list.Resources) < 3 {
		t.Fatalf("expected at least 3 static resources, got %d", len(list.Resources))
	}

	templates, err := session.ListResourceTemplates(ctx, &sdkmcp.ListResourceTemplatesParams{})
	if err != nil {
		t.Fatalf("list templates failed: %v", err)
	}
	if len(templates.ResourceTemplates) < 3 {
		t.Fatalf("expected at least 3 resource templates, got %d", len(templates.ResourceTemplates))
	}

	readRes, err := session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "market://supported-symbols"})
	if err != nil {
		t.Fatalf("read static resource failed: %v", err)
	}
	var symbols []string
	if err := decodeResourceJSON(readRes, &symbols); err != nil {
		t.Fatalf("decode symbols failed: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("expected supported symbols payload")
	}

	readRes, err = session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "signals://latest?symbol=BTC&risk=2&limit=10"})
	if err != nil {
		t.Fatalf("read signals resource failed: %v", err)
	}
	var out signalsListOutput
	if err := decodeResourceJSON(readRes, &out); err != nil {
		t.Fatalf("decode signal output failed: %v", err)
	}
	if len(out.Signals) == 0 {
		t.Fatal("expected signals payload")
	}
	if signals.lastFilter.Symbol != "BTC" {
		t.Fatalf("expected filter symbol BTC, got %s", signals.lastFilter.Symbol)
	}
	if signals.lastFilter.Limit != 10 {
		t.Fatalf("expected filter limit 10, got %d", signals.lastFilter.Limit)
	}
}

func TestRemovedSignalImageResource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv, _, _ := testServer()
	session, shutdown, err := connectInMemory(ctx, srv)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer shutdown()
	defer session.Close()

	_, err = session.ReadResource(ctx, &sdkmcp.ReadResourceParams{URI: "signal-image://2"})
	if err == nil {
		t.Fatal("expected resource not found error for signal-image://2")
	}
}
