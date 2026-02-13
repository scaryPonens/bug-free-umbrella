package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Tab bar styles
	TabStyle       = lipgloss.NewStyle().Padding(0, 2)
	ActiveTabStyle = TabStyle.Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4"))
	InactiveTabStyle = TabStyle.
				Foreground(lipgloss.Color("#888888"))

	// Price colors
	PriceUpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	PriceDownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	PriceZeroStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// Signal direction colors
	DirectionLongStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
	DirectionShortStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	DirectionHoldStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	// Risk level colors
	RiskLowStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	RiskMedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	RiskHighStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))

	// General styles
	HeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA"))
	SubtextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	BorderStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#555555"))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	SpinnerColor = lipgloss.Color("#7D56F4")

	// Chat styles
	UserMsgStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	AssistantMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))

	// Heat map colors
	HeatGreen   = lipgloss.Color("#00FF00")
	HeatRed     = lipgloss.Color("#FF0000")
	HeatNeutral = lipgloss.Color("#555555")

	// Accuracy bar colors
	AccuracyGoodStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	AccuracyOkStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	AccuracyBadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
)
