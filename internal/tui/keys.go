package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines key bindings used across the TUI.
type KeyMap struct {
	Tab      key.Binding
	ShiftTab key.Binding
	Quit     key.Binding
	Refresh  key.Binding

	// Signal explorer filters
	FilterSymbol    key.Binding
	FilterRisk      key.Binding
	FilterIndicator key.Binding

	// Backtest view toggle
	ToggleView key.Binding
}

// DefaultKeyMap provides the default key bindings for the TUI.
var DefaultKeyMap = KeyMap{
	Tab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	ShiftTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Refresh:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh")),

	FilterSymbol:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle symbol")),
	FilterRisk:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "cycle risk")),
	FilterIndicator: key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "cycle indicator")),

	ToggleView: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "toggle view")),
}
