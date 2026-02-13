package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"

	"github.com/charmbracelet/lipgloss"
)

// FormatPrice renders a price snapshot as a single line.
func FormatPrice(p *domain.PriceSnapshot) string {
	changeStyle := PriceZeroStyle
	if p.Change24hPct > 0 {
		changeStyle = PriceUpStyle
	} else if p.Change24hPct < 0 {
		changeStyle = PriceDownStyle
	}

	sign := ""
	if p.Change24hPct > 0 {
		sign = "+"
	}

	return fmt.Sprintf("%-6s %10s  %s  Vol: %s",
		p.Symbol,
		formatUSD(p.PriceUSD),
		changeStyle.Render(fmt.Sprintf("%s%.1f%%", sign, p.Change24hPct)),
		formatVolume(p.Volume24h),
	)
}

// FormatSignal renders a signal as a single line.
func FormatSignal(s domain.Signal) string {
	dirStyle := DirectionHoldStyle
	switch s.Direction {
	case domain.DirectionLong:
		dirStyle = DirectionLongStyle
	case domain.DirectionShort:
		dirStyle = DirectionShortStyle
	}

	riskStyle := RiskLowStyle
	if s.Risk >= 4 {
		riskStyle = RiskHighStyle
	} else if s.Risk >= 3 {
		riskStyle = RiskMedStyle
	}

	return fmt.Sprintf("#%-4d %-5s %-3s %-10s %s risk %s  %s",
		s.ID,
		s.Symbol,
		s.Interval,
		strings.ToUpper(s.Indicator),
		dirStyle.Render(strings.ToUpper(string(s.Direction))),
		riskStyle.Render(fmt.Sprintf("%d", s.Risk)),
		s.Timestamp.Format(time.RFC822),
	)
}

// RenderHeatMap renders a colored grid showing 24h change for each symbol.
func RenderHeatMap(prices []*domain.PriceSnapshot, width int) string {
	if len(prices) == 0 {
		return SubtextStyle.Render("No price data")
	}

	cellWidth := 8
	cols := width / cellWidth
	if cols < 1 {
		cols = 1
	}

	var rows []string
	var row []string
	for i, p := range prices {
		bg := HeatNeutral
		if p.Change24hPct > 0 {
			bg = heatColorScale(p.Change24hPct, 10, HeatGreen)
		} else if p.Change24hPct < 0 {
			bg = heatColorScale(-p.Change24hPct, 10, HeatRed)
		}

		cell := lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Width(cellWidth - 1).
			Align(lipgloss.Center).
			Render(p.Symbol)

		row = append(row, cell)
		if (i+1)%cols == 0 || i == len(prices)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, row...))
			row = nil
		}
	}

	return strings.Join(rows, "\n")
}

// RenderBarChart renders an ASCII bar chart of accuracy values.
func RenderBarChart(label string, accuracy float64, barWidth int) string {
	if barWidth <= 0 {
		barWidth = 20
	}
	filled := int(math.Round(accuracy * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	style := AccuracyGoodStyle
	if accuracy < 0.6 {
		style = AccuracyBadStyle
	} else if accuracy < 0.75 {
		style = AccuracyOkStyle
	}

	bar := style.Render(strings.Repeat("█", filled)) + SubtextStyle.Render(strings.Repeat("░", empty))
	return fmt.Sprintf("%-20s %s %.1f%%", label, bar, accuracy*100)
}

// heatColorScale produces a color scaled by magnitude.
func heatColorScale(magnitude, maxMagnitude float64, baseColor lipgloss.Color) lipgloss.Color {
	intensity := magnitude / maxMagnitude
	if intensity > 1 {
		intensity = 1
	}
	if intensity < 0.1 {
		return HeatNeutral
	}
	return baseColor
}

func formatUSD(v float64) string {
	if v >= 1000 {
		return "$" + addCommas(fmt.Sprintf("%.0f", v))
	}
	if v >= 1 {
		return fmt.Sprintf("$%.2f", v)
	}
	return fmt.Sprintf("$%.4f", v)
}

func addCommas(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var result strings.Builder
	for i, ch := range s {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteByte(',')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func formatVolume(v float64) string {
	switch {
	case v >= 1e12:
		return fmt.Sprintf("$%.1fT", v/1e12)
	case v >= 1e9:
		return fmt.Sprintf("$%.1fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("$%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("$%.1fK", v/1e3)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
}
