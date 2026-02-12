package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"bug-free-umbrella/internal/domain"

	tele "gopkg.in/telebot.v3"
)

func TestParseAlertMode(t *testing.T) {
	mode, err := parseAlertMode(nil)
	if err != nil || mode != "status" {
		t.Fatalf("expected default status mode, got mode=%q err=%v", mode, err)
	}

	mode, err = parseAlertMode([]string{"on"})
	if err != nil || mode != "on" {
		t.Fatalf("expected on mode, got mode=%q err=%v", mode, err)
	}

	mode, err = parseAlertMode([]string{"OFF"})
	if err != nil || mode != "off" {
		t.Fatalf("expected off mode, got mode=%q err=%v", mode, err)
	}

	if _, err := parseAlertMode([]string{"nope"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestAlertDispatcherNotifySignals(t *testing.T) {
	sender := &fakeSender{}
	dispatcher := NewAlertDispatcher(sender)

	if !dispatcher.Subscribe(10) {
		t.Fatal("expected initial subscribe to return true")
	}
	if !dispatcher.Subscribe(20) {
		t.Fatal("expected initial subscribe to return true")
	}
	if dispatcher.Subscribe(10) {
		t.Fatal("expected duplicate subscribe to return false")
	}

	signals := []domain.Signal{{
		Symbol:    "BTC",
		Interval:  "1h",
		Indicator: domain.IndicatorRSI,
		Direction: domain.DirectionLong,
		Risk:      domain.RiskLevel2,
		Timestamp: time.Unix(0, 0).UTC(),
	}}

	if err := dispatcher.NotifySignals(context.Background(), signals); err != nil {
		t.Fatalf("unexpected notify error: %v", err)
	}
	if len(sender.messages[10]) != 1 || len(sender.messages[20]) != 1 {
		t.Fatalf("expected one message per subscriber, got %+v", sender.messages)
	}
	if !strings.Contains(sender.messages[10][0], "BTC 1h RSI LONG") {
		t.Fatalf("unexpected alert body: %s", sender.messages[10][0])
	}
}

func TestAlertDispatcherUnsubscribe(t *testing.T) {
	sender := &fakeSender{}
	dispatcher := NewAlertDispatcher(sender)

	dispatcher.Subscribe(10)
	if !dispatcher.Unsubscribe(10) {
		t.Fatal("expected unsubscribe to return true")
	}
	if dispatcher.Unsubscribe(10) {
		t.Fatal("expected second unsubscribe to return false")
	}

	signals := []domain.Signal{{
		Symbol:    "ETH",
		Interval:  "4h",
		Indicator: domain.IndicatorMACD,
		Direction: domain.DirectionShort,
		Risk:      domain.RiskLevel4,
		Timestamp: time.Now().UTC(),
	}}
	if err := dispatcher.NotifySignals(context.Background(), signals); err != nil {
		t.Fatalf("unexpected notify error: %v", err)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("expected zero outgoing messages, got %+v", sender.messages)
	}
}

type fakeSender struct {
	messages map[int64][]string
}

func (f *fakeSender) Send(to tele.Recipient, what interface{}, opts ...interface{}) (*tele.Message, error) {
	if f.messages == nil {
		f.messages = make(map[int64][]string)
	}

	chat, ok := to.(*tele.Chat)
	if !ok {
		return nil, fmt.Errorf("unexpected recipient type %T", to)
	}
	f.messages[chat.ID] = append(f.messages[chat.ID], fmt.Sprint(what))
	return &tele.Message{}, nil
}
