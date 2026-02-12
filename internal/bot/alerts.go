package bot

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"bug-free-umbrella/internal/domain"

	tele "gopkg.in/telebot.v3"
)

type messageSender interface {
	Send(to tele.Recipient, what interface{}, opts ...interface{}) (*tele.Message, error)
}

// AlertDispatcher broadcasts newly-generated signals to subscribed chats.
type AlertDispatcher struct {
	sender messageSender

	mu          sync.RWMutex
	subscribers map[int64]struct{}
}

func NewAlertDispatcher(sender messageSender) *AlertDispatcher {
	return &AlertDispatcher{
		sender:      sender,
		subscribers: make(map[int64]struct{}),
	}
}

func (d *AlertDispatcher) Subscribe(chatID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.subscribers[chatID]; exists {
		return false
	}
	d.subscribers[chatID] = struct{}{}
	return true
}

func (d *AlertDispatcher) Unsubscribe(chatID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.subscribers[chatID]; !exists {
		return false
	}
	delete(d.subscribers, chatID)
	return true
}

func (d *AlertDispatcher) IsSubscribed(chatID int64) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.subscribers[chatID]
	return exists
}

func (d *AlertDispatcher) SubscriberCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.subscribers)
}

func (d *AlertDispatcher) NotifySignals(ctx context.Context, signals []domain.Signal) error {
	_ = ctx
	if d == nil || d.sender == nil || len(signals) == 0 {
		return nil
	}

	chatIDs := d.snapshotSubscribers()
	if len(chatIDs) == 0 {
		return nil
	}

	msg := formatAlertMessage(signals)
	var failures []string
	for _, chatID := range chatIDs {
		if _, err := d.sender.Send(&tele.Chat{ID: chatID}, msg); err != nil {
			failures = append(failures, fmt.Sprintf("chat %d: %v", chatID, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed sending %d alerts: %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

func (d *AlertDispatcher) snapshotSubscribers() []int64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	chatIDs := make([]int64, 0, len(d.subscribers))
	for chatID := range d.subscribers {
		chatIDs = append(chatIDs, chatID)
	}
	sort.Slice(chatIDs, func(i, j int) bool { return chatIDs[i] < chatIDs[j] })
	return chatIDs
}

func parseAlertMode(args []string) (string, error) {
	if len(args) == 0 {
		return "status", nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on":
		return "on", nil
	case "off":
		return "off", nil
	case "status":
		return "status", nil
	default:
		return "", fmt.Errorf("invalid mode")
	}
}

func formatAlertMessage(signals []domain.Signal) string {
	lines := make([]string, 0, len(signals)+1)
	lines = append(lines, "Proactive signal alert:")
	for _, s := range signals {
		lines = append(lines, formatSignal(s))
	}
	return strings.Join(lines, "\n")
}
