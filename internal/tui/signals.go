package tui

import (
	"context"
	"fmt"
	"strings"

	"bug-free-umbrella/internal/domain"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Signal explorer message types.
type filteredSignalsMsg []domain.Signal
type filteredSignalsErrMsg struct{ err error }

var (
	symbolOptions = []string{
		"ALL", "BTC", "ETH", "SOL", "XRP", "ADA", "DOGE", "DOT", "AVAX", "LINK", "MATIC",
	}
	riskOptions = []string{"ALL", "1", "2", "3", "4", "5"}
	indicatorOptions = []string{
		"ALL", "rsi", "macd", "bollinger", "volume_zscore",
		"ml_logreg_up4h", "ml_xgboost_up4h", "ml_ensemble_up4h",
		"fund_sentiment_composite",
	}
)

// SignalExplorerModel is the Bubble Tea model for the signal explorer screen.
type SignalExplorerModel struct {
	services     Services
	signals      []domain.Signal
	symbolIdx    int
	riskIdx      int
	indicatorIdx int
	scrollOffset int
	loading      bool
	err          error
	width        int
	height       int
}

// NewSignalExplorerModel creates a new signal explorer model.
func NewSignalExplorerModel(svc Services) SignalExplorerModel {
	return SignalExplorerModel{
		services: svc,
		loading:  true,
	}
}

// Init fires initial signal fetch.
func (m SignalExplorerModel) Init() tea.Cmd {
	return m.fetchSignalsCmd()
}

// Update handles incoming messages.
func (m SignalExplorerModel) Update(msg tea.Msg) (SignalExplorerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case filteredSignalsMsg:
		m.signals = []domain.Signal(msg)
		m.loading = false
		m.scrollOffset = 0
		m.err = nil
		return m, nil

	case filteredSignalsErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, DefaultKeyMap.FilterSymbol):
			m.symbolIdx = (m.symbolIdx + 1) % len(symbolOptions)
			m.loading = true
			return m, m.fetchSignalsCmd()

		case key.Matches(msg, DefaultKeyMap.FilterRisk):
			m.riskIdx = (m.riskIdx + 1) % len(riskOptions)
			m.loading = true
			return m, m.fetchSignalsCmd()

		case key.Matches(msg, DefaultKeyMap.FilterIndicator):
			m.indicatorIdx = (m.indicatorIdx + 1) % len(indicatorOptions)
			m.loading = true
			return m, m.fetchSignalsCmd()

		case key.Matches(msg, DefaultKeyMap.Refresh):
			m.loading = true
			return m, m.fetchSignalsCmd()

		case msg.String() == "j" || msg.String() == "down":
			maxVisible := m.visibleRows()
			if m.scrollOffset < len(m.signals)-maxVisible {
				m.scrollOffset++
			}
			return m, nil

		case msg.String() == "k" || msg.String() == "up":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		}
	}

	return m, nil
}

// View renders the signal explorer.
func (m SignalExplorerModel) View() string {
	var sections []string

	// Header
	sections = append(sections, HeaderStyle.Render("  Signal Explorer"))
	sections = append(sections, "")

	// Filter chips
	sections = append(sections, m.renderFilters())
	sections = append(sections, SubtextStyle.Render(strings.Repeat("─", m.width-2)))

	if m.loading {
		sections = append(sections, SubtextStyle.Render("  Loading..."))
		return strings.Join(sections, "\n")
	}

	if m.err != nil {
		sections = append(sections, ErrorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		return strings.Join(sections, "\n")
	}

	if len(m.signals) == 0 {
		sections = append(sections, SubtextStyle.Render("  No signals match the current filters"))
		return strings.Join(sections, "\n")
	}

	// Table header
	sections = append(sections, SubtextStyle.Render(
		fmt.Sprintf("  %-5s %-6s %-4s %-12s %-6s %-5s  %s",
			"ID", "Symbol", "Int", "Indicator", "Dir", "Risk", "Time"),
	))

	// Table rows
	maxVisible := m.visibleRows()
	end := m.scrollOffset + maxVisible
	if end > len(m.signals) {
		end = len(m.signals)
	}

	for i := m.scrollOffset; i < end; i++ {
		sections = append(sections, "  "+FormatSignal(m.signals[i]))
	}

	// Scroll indicator
	if len(m.signals) > maxVisible {
		sections = append(sections, SubtextStyle.Render(
			fmt.Sprintf("  Showing %d-%d of %d (j/k to scroll)", m.scrollOffset+1, end, len(m.signals)),
		))
	}

	// Help
	sections = append(sections, "")
	sections = append(sections, SubtextStyle.Render("  [s] symbol  [r] risk  [i] indicator  [R] refresh  [j/k] scroll"))

	return strings.Join(sections, "\n")
}

// SetSize updates the model dimensions.
func (m *SignalExplorerModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// FilterState returns current filter indices (for testing).
func (m SignalExplorerModel) FilterState() (symbolIdx, riskIdx, indicatorIdx int) {
	return m.symbolIdx, m.riskIdx, m.indicatorIdx
}

// SignalCount returns the number of loaded signals (for testing).
func (m SignalExplorerModel) SignalCount() int { return len(m.signals) }

func (m SignalExplorerModel) renderFilters() string {
	symbolChip := m.renderChip("Symbol", symbolOptions, m.symbolIdx)
	riskChip := m.renderChip("Risk", riskOptions, m.riskIdx)
	indChip := m.renderChip("Type", indicatorOptions, m.indicatorIdx)
	return "  " + lipgloss.JoinHorizontal(lipgloss.Top, symbolChip, "  ", riskChip, "  ", indChip)
}

func (m SignalExplorerModel) renderChip(label string, options []string, active int) string {
	var parts []string
	parts = append(parts, SubtextStyle.Render(label+": "))
	for i, opt := range options {
		display := strings.ToUpper(opt)
		if len(display) > 6 {
			display = display[:6]
		}
		if i == active {
			parts = append(parts, ActiveTabStyle.Render(display))
		} else {
			parts = append(parts, SubtextStyle.Render(display))
		}
		parts = append(parts, " ")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m SignalExplorerModel) buildFilter() domain.SignalFilter {
	filter := domain.SignalFilter{Limit: 100}

	if m.symbolIdx > 0 && m.symbolIdx < len(symbolOptions) {
		filter.Symbol = symbolOptions[m.symbolIdx]
	}

	if m.riskIdx > 0 && m.riskIdx < len(riskOptions) {
		risk := domain.RiskLevel(m.riskIdx)
		filter.Risk = &risk
	}

	if m.indicatorIdx > 0 && m.indicatorIdx < len(indicatorOptions) {
		filter.Indicator = indicatorOptions[m.indicatorIdx]
	}

	return filter
}

func (m SignalExplorerModel) fetchSignalsCmd() tea.Cmd {
	filter := m.buildFilter()
	return func() tea.Msg {
		if m.services.Signals == nil {
			return filteredSignalsErrMsg{err: fmt.Errorf("signal service not available")}
		}
		signals, err := m.services.Signals.ListSignals(context.Background(), filter)
		if err != nil {
			return filteredSignalsErrMsg{err: err}
		}
		return filteredSignalsMsg(signals)
	}
}

func (m SignalExplorerModel) visibleRows() int {
	// Account for header, filters, table header, help footer
	available := m.height - 10
	if available < 5 {
		return 5
	}
	return available
}
