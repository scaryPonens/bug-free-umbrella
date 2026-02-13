package tui

import (
	"context"
	"fmt"
	"strings"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Backtest message types.
type backtestSummaryMsg []repository.DailyAccuracy
type backtestDailyMsg []repository.DailyAccuracy
type backtestPredictionsMsg []domain.MLPrediction
type backtestErrMsg struct{ err error }

const (
	backtestViewAccuracy    = 0
	backtestViewPredictions = 1
)

// BacktestModel is the Bubble Tea model for the backtest viewer screen.
type BacktestModel struct {
	services    Services
	summary     []repository.DailyAccuracy
	daily       []repository.DailyAccuracy
	predictions []domain.MLPrediction
	activeView  int
	loading     bool
	err         error
	width       int
	height      int
}

// NewBacktestModel creates a new backtest viewer model.
func NewBacktestModel(svc Services) BacktestModel {
	return BacktestModel{
		services: svc,
		loading:  true,
	}
}

// Init fires initial data fetch commands.
func (m BacktestModel) Init() tea.Cmd {
	return tea.Batch(
		m.fetchSummaryCmd(),
		m.fetchDailyCmd(),
		m.fetchPredictionsCmd(),
	)
}

// Update handles incoming messages.
func (m BacktestModel) Update(msg tea.Msg) (BacktestModel, tea.Cmd) {
	switch msg := msg.(type) {
	case backtestSummaryMsg:
		m.summary = []repository.DailyAccuracy(msg)
		m.loading = false
		return m, nil

	case backtestDailyMsg:
		m.daily = []repository.DailyAccuracy(msg)
		return m, nil

	case backtestPredictionsMsg:
		m.predictions = []domain.MLPrediction(msg)
		return m, nil

	case backtestErrMsg:
		m.err = msg.err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, DefaultKeyMap.ToggleView):
			m.activeView = 1 - m.activeView
			return m, nil

		case key.Matches(msg, DefaultKeyMap.Refresh):
			m.loading = true
			return m, tea.Batch(
				m.fetchSummaryCmd(),
				m.fetchDailyCmd(),
				m.fetchPredictionsCmd(),
			)
		}
	}

	return m, nil
}

// View renders the backtest viewer.
func (m BacktestModel) View() string {
	var sections []string

	// Header with view toggle
	viewLabel := "[Accuracy]  Predictions"
	if m.activeView == backtestViewPredictions {
		viewLabel = " Accuracy  [Predictions]"
	}
	sections = append(sections, HeaderStyle.Render("  Backtest Viewer")+"  "+SubtextStyle.Render(viewLabel))
	sections = append(sections, "")

	if m.loading {
		sections = append(sections, SubtextStyle.Render("  Loading backtest data..."))
		return strings.Join(sections, "\n")
	}

	if m.err != nil {
		sections = append(sections, ErrorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		return strings.Join(sections, "\n")
	}

	if m.activeView == backtestViewAccuracy {
		sections = append(sections, m.renderAccuracyView()...)
	} else {
		sections = append(sections, m.renderPredictionsView()...)
	}

	sections = append(sections, "")
	sections = append(sections, SubtextStyle.Render("  [v] toggle view  [R] refresh"))

	return strings.Join(sections, "\n")
}

// SetSize updates the model dimensions.
func (m *BacktestModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ActiveView returns the current view index (for testing).
func (m BacktestModel) ActiveView() int { return m.activeView }

// HasData returns whether any backtest data is loaded.
func (m BacktestModel) HasData() bool {
	return len(m.summary) > 0 || len(m.daily) > 0 || len(m.predictions) > 0
}

func (m BacktestModel) renderAccuracyView() []string {
	var lines []string

	if len(m.summary) == 0 && len(m.daily) == 0 {
		lines = append(lines, SubtextStyle.Render("  No backtest data available. Enable ML models (Phase 6) to see prediction accuracy."))
		return lines
	}

	// Overall model accuracy
	if len(m.summary) > 0 {
		lines = append(lines, HeaderStyle.Render("  Model Accuracy (All-Time)"))
		lines = append(lines, "")

		barWidth := m.width/3 - 5
		if barWidth < 10 {
			barWidth = 10
		}
		if barWidth > 30 {
			barWidth = 30
		}

		for _, s := range m.summary {
			bar := RenderBarChart(s.ModelKey, s.Accuracy, barWidth)
			lines = append(lines, fmt.Sprintf("  %s  (%d)", bar, s.Total))
		}
		lines = append(lines, "")
	}

	// Daily breakdown
	if len(m.daily) > 0 {
		lines = append(lines, HeaderStyle.Render("  Daily Accuracy (Last 30 Days)"))
		lines = append(lines, "")

		barWidth := m.width/3 - 5
		if barWidth < 10 {
			barWidth = 10
		}
		if barWidth > 30 {
			barWidth = 30
		}

		count := len(m.daily)
		maxRows := m.height - 15
		if maxRows < 5 {
			maxRows = 5
		}
		if count > maxRows {
			count = maxRows
		}

		for i := 0; i < count; i++ {
			d := m.daily[i]
			label := d.DayUTC.Format("2006-01-02")
			bar := RenderBarChart(label, d.Accuracy, barWidth)
			lines = append(lines, fmt.Sprintf("  %s  (%d/%d)", bar, d.Correct, d.Total))
		}
	}

	return lines
}

func (m BacktestModel) renderPredictionsView() []string {
	var lines []string

	if len(m.predictions) == 0 {
		lines = append(lines, SubtextStyle.Render("  No resolved predictions available."))
		return lines
	}

	lines = append(lines, HeaderStyle.Render("  Recent Resolved Predictions"))
	lines = append(lines, "")
	lines = append(lines, SubtextStyle.Render(
		fmt.Sprintf("  %-6s %-4s %-18s %-6s %-5s %-8s %-8s",
			"Symbol", "Int", "Model", "Dir", "Risk", "Correct", "Return"),
	))
	lines = append(lines, SubtextStyle.Render("  "+strings.Repeat("─", 65)))

	maxRows := m.height - 10
	if maxRows < 5 {
		maxRows = 5
	}
	count := len(m.predictions)
	if count > maxRows {
		count = maxRows
	}

	for i := 0; i < count; i++ {
		p := m.predictions[i]

		correctStr := "?"
		if p.IsCorrect != nil {
			if *p.IsCorrect {
				correctStr = PriceUpStyle.Render("YES")
			} else {
				correctStr = PriceDownStyle.Render("NO")
			}
		}

		returnStr := "n/a"
		if p.RealizedReturn != nil {
			sign := ""
			if *p.RealizedReturn > 0 {
				sign = "+"
			}
			returnStr = fmt.Sprintf("%s%.2f%%", sign, *p.RealizedReturn*100)
		}

		dirStyle := DirectionHoldStyle
		switch p.Direction {
		case domain.DirectionLong:
			dirStyle = DirectionLongStyle
		case domain.DirectionShort:
			dirStyle = DirectionShortStyle
		}

		lines = append(lines, fmt.Sprintf("  %-6s %-4s %-18s %s %-5d %-8s %-8s",
			p.Symbol,
			p.Interval,
			p.ModelKey,
			dirStyle.Render(fmt.Sprintf("%-6s", strings.ToUpper(string(p.Direction)))),
			p.Risk,
			correctStr,
			returnStr,
		))
	}

	if len(m.predictions) > maxRows {
		lines = append(lines, SubtextStyle.Render(
			fmt.Sprintf("  Showing %d of %d predictions", count, len(m.predictions)),
		))
	}

	return lines
}

func (m BacktestModel) fetchSummaryCmd() tea.Cmd {
	return func() tea.Msg {
		if m.services.Backtest == nil {
			return backtestErrMsg{err: fmt.Errorf("backtest service not available")}
		}
		summary, err := m.services.Backtest.GetAccuracySummary(context.Background())
		if err != nil {
			return backtestErrMsg{err: err}
		}
		return backtestSummaryMsg(summary)
	}
}

func (m BacktestModel) fetchDailyCmd() tea.Cmd {
	return func() tea.Msg {
		if m.services.Backtest == nil {
			return nil
		}
		// Aggregate all models into one daily list
		daily, err := m.services.Backtest.GetDailyAccuracy(context.Background(), "", 30)
		if err != nil {
			return nil // Non-critical
		}
		return backtestDailyMsg(daily)
	}
}

func (m BacktestModel) fetchPredictionsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.services.Backtest == nil {
			return nil
		}
		preds, err := m.services.Backtest.ListRecentPredictions(context.Background(), 50)
		if err != nil {
			return nil // Non-critical
		}
		return backtestPredictionsMsg(preds)
	}
}
