package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Dashboard message types.
type pricesMsg []*domain.PriceSnapshot
type pricesErrMsg struct{ err error }
type signalsMsg []domain.Signal
type signalsErrMsg struct{ err error }
type dashTickMsg time.Time

// DashboardModel is the Bubble Tea model for the live dashboard screen.
type DashboardModel struct {
	services Services
	prices   []*domain.PriceSnapshot
	signals  []domain.Signal
	loading  bool
	err      error
	width    int
	height   int
}

// NewDashboardModel creates a new dashboard model.
func NewDashboardModel(svc Services) DashboardModel {
	return DashboardModel{
		services: svc,
		loading:  true,
	}
}

// Init fires initial data fetch commands.
func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchPricesCmd(),
		m.fetchSignalsCmd(),
		m.tickCmd(),
	)
}

// Update handles incoming messages.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case pricesMsg:
		m.prices = []*domain.PriceSnapshot(msg)
		m.loading = false
		m.err = nil
		return m, nil

	case pricesErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case signalsMsg:
		m.signals = []domain.Signal(msg)
		return m, nil

	case signalsErrMsg:
		// Non-critical; prices are more important.
		return m, nil

	case dashTickMsg:
		return m, tea.Batch(
			m.fetchPricesCmd(),
			m.fetchSignalsCmd(),
			m.tickCmd(),
		)
	}

	return m, nil
}

// View renders the dashboard.
func (m DashboardModel) View() string {
	if m.loading && len(m.prices) == 0 {
		return SubtextStyle.Render("Loading prices...")
	}
	if m.err != nil && len(m.prices) == 0 {
		return ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	var sections []string

	// Price table + Heat map side by side
	priceTable := m.renderPriceTable()
	heatMap := m.renderHeatMapSection()

	priceWidth := m.width*2/3 - 2
	if priceWidth < 40 {
		priceWidth = 40
	}
	heatWidth := m.width - priceWidth - 4
	if heatWidth < 15 {
		heatWidth = 15
	}

	priceBox := BorderStyle.Width(priceWidth).Render(priceTable)
	heatBox := BorderStyle.Width(heatWidth).Render(heatMap)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, priceBox, heatBox)
	sections = append(sections, topRow)

	// Active signals
	signalSection := m.renderSignals()
	signalBox := BorderStyle.Width(m.width - 2).Render(signalSection)
	sections = append(sections, signalBox)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// SetSize updates the model dimensions.
func (m *DashboardModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Prices returns the current prices (for testing).
func (m DashboardModel) Prices() []*domain.PriceSnapshot { return m.prices }

// Signals returns the current signals (for testing).
func (m DashboardModel) Signals() []domain.Signal { return m.signals }

func (m DashboardModel) renderPriceTable() string {
	header := HeaderStyle.Render("  Live Prices")
	var lines []string
	lines = append(lines, header)
	lines = append(lines, SubtextStyle.Render("  Symbol       Price      24h       Volume"))
	lines = append(lines, SubtextStyle.Render(strings.Repeat("─", 55)))

	for _, p := range m.prices {
		lines = append(lines, "  "+FormatPrice(p))
	}

	if len(m.prices) == 0 {
		lines = append(lines, SubtextStyle.Render("  No price data available"))
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) renderHeatMapSection() string {
	header := HeaderStyle.Render("  Heat Map")
	heatWidth := m.width/3 - 4
	if heatWidth < 15 {
		heatWidth = 15
	}
	heatMap := RenderHeatMap(m.prices, heatWidth)
	return header + "\n" + heatMap
}

func (m DashboardModel) renderSignals() string {
	header := HeaderStyle.Render("  Active Signals")
	var lines []string
	lines = append(lines, header)

	count := len(m.signals)
	if count > 10 {
		count = 10
	}

	for i := 0; i < count; i++ {
		lines = append(lines, "  "+FormatSignal(m.signals[i]))
	}

	if len(m.signals) == 0 {
		lines = append(lines, SubtextStyle.Render("  No active signals"))
	}

	return strings.Join(lines, "\n")
}

func (m DashboardModel) fetchPricesCmd() tea.Cmd {
	return func() tea.Msg {
		if m.services.Prices == nil {
			return pricesErrMsg{err: fmt.Errorf("price service not available")}
		}
		prices, err := m.services.Prices.GetCurrentPrices(context.Background())
		if err != nil {
			return pricesErrMsg{err: err}
		}
		return pricesMsg(prices)
	}
}

func (m DashboardModel) fetchSignalsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.services.Signals == nil {
			return signalsErrMsg{err: fmt.Errorf("signal service not available")}
		}
		signals, err := m.services.Signals.ListSignals(context.Background(), domain.SignalFilter{Limit: 10})
		if err != nil {
			return signalsErrMsg{err: err}
		}
		return signalsMsg(signals)
	}
}

func (m DashboardModel) tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return dashTickMsg(t)
	})
}
