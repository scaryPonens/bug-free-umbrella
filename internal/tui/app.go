package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab represents a screen tab in the TUI.
type Tab int

const (
	TabDashboard Tab = iota
	TabChat
	TabSignals
	TabBacktest
)

var tabNames = []string{"1:Dashboard", "2:Chat", "3:Signals", "4:Backtest"}

// AppModel is the root Bubble Tea model that manages tab navigation and child screens.
type AppModel struct {
	services  Services
	activeTab Tab
	dashboard DashboardModel
	chat      ChatModel
	signals   SignalExplorerModel
	backtest  BacktestModel
	width     int
	height    int
	quitting  bool
}

// NewAppModel creates the root application model with all child screens.
func NewAppModel(svc Services) AppModel {
	return AppModel{
		services:  svc,
		activeTab: TabDashboard,
		dashboard: NewDashboardModel(svc),
		chat:      NewChatModel(svc),
		signals:   NewSignalExplorerModel(svc),
		backtest:  NewBacktestModel(svc),
	}
}

// Init initializes all child models.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.dashboard.Init(),
		m.chat.Init(),
		m.signals.Init(),
		m.backtest.Init(),
	)
}

// Update handles incoming messages, routing to the active tab.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.propagateSize()
		return m, nil

	case tea.KeyMsg:
		// Global key bindings (except in chat when input is focused)
		if m.activeTab != TabChat || msg.Type == tea.KeyTab || msg.Type == tea.KeyShiftTab ||
			msg.String() == "ctrl+c" || (msg.String() >= "1" && msg.String() <= "4") {

			switch {
			case key.Matches(msg, DefaultKeyMap.Quit):
				// Don't quit on 'q' in chat mode
				if m.activeTab == TabChat && msg.String() == "q" {
					break
				}
				m.quitting = true
				return m, tea.Quit

			case key.Matches(msg, DefaultKeyMap.Tab):
				m.switchTab(Tab((int(m.activeTab) + 1) % len(tabNames)))
				return m, nil

			case key.Matches(msg, DefaultKeyMap.ShiftTab):
				next := int(m.activeTab) - 1
				if next < 0 {
					next = len(tabNames) - 1
				}
				m.switchTab(Tab(next))
				return m, nil

			case msg.String() == "1":
				m.switchTab(TabDashboard)
				return m, nil
			case msg.String() == "2":
				m.switchTab(TabChat)
				return m, nil
			case msg.String() == "3":
				m.switchTab(TabSignals)
				return m, nil
			case msg.String() == "4":
				m.switchTab(TabBacktest)
				return m, nil
			}
		}
	}

	// Route messages to all child models (they filter relevant ones)
	var cmds []tea.Cmd

	switch msg.(type) {
	case pricesMsg, pricesErrMsg, signalsMsg, signalsErrMsg, dashTickMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		cmds = append(cmds, cmd)

	case filteredSignalsMsg, filteredSignalsErrMsg:
		var cmd tea.Cmd
		m.signals, cmd = m.signals.Update(msg)
		cmds = append(cmds, cmd)

	case backtestSummaryMsg, backtestDailyMsg, backtestPredictionsMsg, backtestErrMsg:
		var cmd tea.Cmd
		m.backtest, cmd = m.backtest.Update(msg)
		cmds = append(cmds, cmd)

	case advisorReplyMsg, advisorErrMsg:
		var cmd tea.Cmd
		m.chat, cmd = m.chat.Update(msg)
		cmds = append(cmds, cmd)

	default:
		// Route keyboard and other messages to active tab only
		switch m.activeTab {
		case TabDashboard:
			var cmd tea.Cmd
			m.dashboard, cmd = m.dashboard.Update(msg)
			cmds = append(cmds, cmd)
		case TabChat:
			var cmd tea.Cmd
			m.chat, cmd = m.chat.Update(msg)
			cmds = append(cmds, cmd)
		case TabSignals:
			var cmd tea.Cmd
			m.signals, cmd = m.signals.Update(msg)
			cmds = append(cmds, cmd)
		case TabBacktest:
			var cmd tea.Cmd
			m.backtest, cmd = m.backtest.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the tab bar and active screen.
func (m AppModel) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	tabBar := m.renderTabBar()

	var content string
	switch m.activeTab {
	case TabDashboard:
		content = m.dashboard.View()
	case TabChat:
		content = m.chat.View()
	case TabSignals:
		content = m.signals.View()
	case TabBacktest:
		content = m.backtest.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, tabBar, content)
}

// SetSize updates dimensions on the root model and propagates to children.
func (m *AppModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.propagateSize()
}

// ActiveTab returns the currently active tab (for testing).
func (m AppModel) ActiveTab() Tab { return m.activeTab }

func (m *AppModel) switchTab(tab Tab) {
	if tab == TabChat && m.activeTab != TabChat {
		m.chat.Focus()
	} else if m.activeTab == TabChat && tab != TabChat {
		m.chat.Blur()
	}
	m.activeTab = tab
}

func (m *AppModel) propagateSize() {
	contentHeight := m.height - 2 // account for tab bar
	m.dashboard.SetSize(m.width, contentHeight)
	m.chat.SetSize(m.width, contentHeight)
	m.signals.SetSize(m.width, contentHeight)
	m.backtest.SetSize(m.width, contentHeight)
}

func (m AppModel) renderTabBar() string {
	var tabs []string
	for i, name := range tabNames {
		if Tab(i) == m.activeTab {
			tabs = append(tabs, ActiveTabStyle.Render(name))
		} else {
			tabs = append(tabs, InactiveTabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
