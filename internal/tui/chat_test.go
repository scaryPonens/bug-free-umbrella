package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestChatModelInitialState(t *testing.T) {
	m := NewChatModel(testServices())
	if m.IsWaiting() {
		t.Fatal("expected not waiting initially")
	}
	if m.MessageCount() != 0 {
		t.Fatalf("expected 0 messages, got %d", m.MessageCount())
	}
}

func TestChatModelSendMessage(t *testing.T) {
	m := NewChatModel(testServices())
	m.SetSize(120, 40)

	// Type a message
	m.input.SetValue("What about BTC?")

	// Press Enter
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !updated.IsWaiting() {
		t.Fatal("expected waiting after sending message")
	}
	if updated.MessageCount() != 1 {
		t.Fatalf("expected 1 message, got %d", updated.MessageCount())
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for advisor call")
	}
}

func TestChatModelReceiveReply(t *testing.T) {
	m := NewChatModel(testServices())
	m.SetSize(120, 40)
	m.waiting = true
	m.messages = append(m.messages, chatMessage{Role: "user", Content: "test"})

	updated, _ := m.Update(advisorReplyMsg("BTC looks bullish"))
	if updated.IsWaiting() {
		t.Fatal("expected not waiting after receiving reply")
	}
	if updated.MessageCount() != 2 {
		t.Fatalf("expected 2 messages, got %d", updated.MessageCount())
	}
}

func TestChatModelAdvisorDisabled(t *testing.T) {
	svc := testServices()
	svc.Advisor = nil
	m := NewChatModel(svc)
	m.SetSize(120, 40)

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view even when advisor is disabled")
	}
}

func TestChatModelEmptyMessageIgnored(t *testing.T) {
	m := NewChatModel(testServices())
	m.SetSize(120, 40)
	m.input.SetValue("")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.IsWaiting() {
		t.Fatal("expected not waiting for empty message")
	}
	if updated.MessageCount() != 0 {
		t.Fatalf("expected 0 messages, got %d", updated.MessageCount())
	}
}
