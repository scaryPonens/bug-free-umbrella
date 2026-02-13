package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Chat message types.
type advisorReplyMsg string
type advisorErrMsg struct{ err error }

type chatMessage struct {
	Role    string
	Content string
	Time    time.Time
}

// ChatModel is the Bubble Tea model for the advisor chat screen.
type ChatModel struct {
	services Services
	messages []chatMessage
	input    textinput.Model
	viewport viewport.Model
	spinner  spinner.Model
	waiting  bool
	err      error
	width    int
	height   int
	ready    bool
}

// NewChatModel creates a new chat model.
func NewChatModel(svc Services) ChatModel {
	ti := textinput.New()
	ti.Placeholder = "Ask about crypto markets..."
	ti.CharLimit = 500
	ti.Width = 60

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(SpinnerColor)

	return ChatModel{
		services: svc,
		input:    ti,
		spinner:  sp,
	}
}

// Init initializes the chat model.
func (m ChatModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles incoming messages.
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case advisorReplyMsg:
		m.messages = append(m.messages, chatMessage{
			Role:    "assistant",
			Content: string(msg),
			Time:    time.Now(),
		})
		m.waiting = false
		m.err = nil
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case advisorErrMsg:
		m.waiting = false
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && !m.waiting {
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				m.messages = append(m.messages, chatMessage{
					Role:    "user",
					Content: text,
					Time:    time.Now(),
				})
				m.input.SetValue("")
				m.waiting = true
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, tea.Batch(
					m.askAdvisorCmd(text),
					m.spinner.Tick,
				)
			}
		}

	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Update text input
	if !m.waiting {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the chat screen.
func (m ChatModel) View() string {
	if m.services.Advisor == nil {
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			HeaderStyle.Render("  Chat with Trading Advisor"),
			"",
			SubtextStyle.Render("  Advisor not available. Set OPENAI_API_KEY to enable."),
		)
	}

	var sections []string

	sections = append(sections, HeaderStyle.Render("  Chat with Trading Advisor"))
	sections = append(sections, SubtextStyle.Render(strings.Repeat("─", m.width-2)))

	// Message viewport
	if !m.ready {
		m.initViewport()
	}
	sections = append(sections, m.viewport.View())

	sections = append(sections, SubtextStyle.Render(strings.Repeat("─", m.width-2)))

	// Input bar
	if m.waiting {
		sections = append(sections, fmt.Sprintf("  %s Thinking...", m.spinner.View()))
	} else {
		if m.err != nil {
			sections = append(sections, ErrorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
		}
		sections = append(sections, "  "+m.input.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// SetSize updates the model dimensions.
func (m *ChatModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.input.Width = w - 6
	if m.ready {
		m.viewport.Width = w - 2
		m.viewport.Height = h - 6 // account for header, borders, input
	}
	m.ready = false // re-initialize viewport on next View
}

// Focus gives focus to the text input.
func (m *ChatModel) Focus() {
	m.input.Focus()
}

// Blur removes focus from the text input.
func (m *ChatModel) Blur() {
	m.input.Blur()
}

// IsWaiting returns whether the model is waiting for a response (for testing).
func (m ChatModel) IsWaiting() bool { return m.waiting }

// MessageCount returns the number of messages (for testing).
func (m ChatModel) MessageCount() int { return len(m.messages) }

func (m *ChatModel) initViewport() {
	vpHeight := m.height - 6
	if vpHeight < 3 {
		vpHeight = 3
	}
	vpWidth := m.width - 2
	if vpWidth < 10 {
		vpWidth = 10
	}
	m.viewport = viewport.New(vpWidth, vpHeight)
	m.viewport.SetContent(m.renderMessages())
	m.ready = true
}

func (m ChatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return SubtextStyle.Render("  Start a conversation by typing a question below.")
	}

	var lines []string
	for _, msg := range m.messages {
		timestamp := SubtextStyle.Render(msg.Time.Format("15:04"))
		switch msg.Role {
		case "user":
			lines = append(lines, fmt.Sprintf("  %s  %s %s",
				timestamp,
				UserMsgStyle.Render("You:"),
				msg.Content,
			))
		case "assistant":
			lines = append(lines, fmt.Sprintf("  %s  %s",
				timestamp,
				AssistantMsgStyle.Render("Advisor:"),
			))
			// Wrap long advisor responses
			for _, line := range strings.Split(msg.Content, "\n") {
				lines = append(lines, "         "+line)
			}
		}
		lines = append(lines, "")
	}

	if m.waiting {
		lines = append(lines, fmt.Sprintf("  %s  %s",
			SubtextStyle.Render(time.Now().Format("15:04")),
			SubtextStyle.Render("Advisor is thinking..."),
		))
	}

	return strings.Join(lines, "\n")
}

func (m ChatModel) askAdvisorCmd(question string) tea.Cmd {
	chatID := m.services.ChatID()
	return func() tea.Msg {
		if m.services.Advisor == nil {
			return advisorErrMsg{err: fmt.Errorf("advisor not available")}
		}
		reply, err := m.services.Advisor.Ask(context.Background(), chatID, question)
		if err != nil {
			return advisorErrMsg{err: err}
		}
		return advisorReplyMsg(reply)
	}
}
