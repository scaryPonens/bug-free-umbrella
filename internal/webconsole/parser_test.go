//go:build legacy_webconsole_emulator
// +build legacy_webconsole_emulator

package webconsole

import "testing"

func TestParseCommandFlags(t *testing.T) {
	parsed, err := ParseCommand("signals --symbol BTC --limit 10 --risk=3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Name != "signals" {
		t.Fatalf("expected signals, got %s", parsed.Name)
	}
	if parsed.Flags["symbol"] != "BTC" {
		t.Fatalf("expected symbol BTC, got %s", parsed.Flags["symbol"])
	}
	if parsed.Flags["limit"] != "10" {
		t.Fatalf("expected limit 10, got %s", parsed.Flags["limit"])
	}
	if parsed.Flags["risk"] != "3" {
		t.Fatalf("expected risk 3, got %s", parsed.Flags["risk"])
	}
}

func TestParseCommandEmpty(t *testing.T) {
	if _, err := ParseCommand("   "); err == nil {
		t.Fatal("expected error")
	}
}
