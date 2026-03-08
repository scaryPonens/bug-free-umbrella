package webconsole

import (
	"context"
	"errors"
	"testing"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"
)

type advisorStub struct {
	reply string
	err   error
	chat  int64
	msg   string
}

func (s *advisorStub) Ask(ctx context.Context, chatID int64, message string) (string, error) {
	s.chat = chatID
	s.msg = message
	if s.err != nil {
		return "", s.err
	}
	return s.reply, nil
}

func TestServiceAsk(t *testing.T) {
	stub := &advisorStub{reply: "ok"}
	svc := NewService(nil, nil, nil, stub)

	reply, err := svc.Ask(context.Background(), "abc", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reply != "ok" {
		t.Fatalf("expected ok, got %s", reply)
	}
	if stub.msg != "hello" {
		t.Fatalf("expected message propagated")
	}
	if stub.chat >= 0 {
		t.Fatalf("expected negative chat id, got %d", stub.chat)
	}
}

func TestServiceAskErrors(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if _, err := svc.Ask(context.Background(), "abc", "hello"); err == nil {
		t.Fatal("expected advisor unavailable error")
	}

	stub := &advisorStub{err: errors.New("boom")}
	svc = NewService(nil, nil, nil, stub)
	if _, err := svc.Ask(context.Background(), "abc", ""); err == nil {
		t.Fatal("expected empty message error")
	}
	if _, err := svc.Ask(context.Background(), "abc", "hello"); err == nil {
		t.Fatal("expected advisor error")
	}
}

func TestServiceDefaults(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if svc.DefaultDashboardSignalLimit() != 10 || svc.DefaultSignalsLimit() != 100 || svc.DefaultBacktestDays() != 30 || svc.DefaultBacktestPredictionsLimit() != 50 {
		t.Fatalf("unexpected defaults")
	}
}

type priceStub struct {
	prices []*domain.PriceSnapshot
}

func (s *priceStub) GetCurrentPrices(context.Context) ([]*domain.PriceSnapshot, error) {
	return s.prices, nil
}

type signalStub struct {
	items []domain.Signal
	last  domain.SignalFilter
}

func (s *signalStub) ListSignals(ctx context.Context, filter domain.SignalFilter) ([]domain.Signal, error) {
	s.last = filter
	return s.items, nil
}

type backtestStub struct {
	summary []repository.DailyAccuracy
	daily   []repository.DailyAccuracy
	preds   []domain.MLPrediction
	model   string
	days    int
	limit   int
}

func (s *backtestStub) GetSummary(context.Context) ([]repository.DailyAccuracy, error) {
	return s.summary, nil
}

func (s *backtestStub) GetDaily(ctx context.Context, model string, days int) ([]repository.DailyAccuracy, error) {
	s.model = model
	s.days = days
	return s.daily, nil
}

func (s *backtestStub) GetPredictions(ctx context.Context, limit int) ([]domain.MLPrediction, error) {
	s.limit = limit
	return s.preds, nil
}

func TestServiceDashboardAndSignalsDefaults(t *testing.T) {
	priceSvc := &priceStub{prices: []*domain.PriceSnapshot{{Symbol: "BTC"}}}
	signalSvc := &signalStub{items: []domain.Signal{{ID: 1, Symbol: "BTC"}}}
	svc := NewService(priceSvc, signalSvc, nil, nil)

	dash, err := svc.GetDashboard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dash.Prices) != 1 || len(dash.Signals) != 1 {
		t.Fatalf("expected dashboard payload")
	}
	if signalSvc.last.Limit != 10 {
		t.Fatalf("expected dashboard default limit 10, got %d", signalSvc.last.Limit)
	}

	_, err = svc.GetSignals(context.Background(), domain.SignalFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signalSvc.last.Limit != 100 {
		t.Fatalf("expected signals default limit 100, got %d", signalSvc.last.Limit)
	}
}

func TestServiceBacktestDefaults(t *testing.T) {
	backtestSvc := &backtestStub{
		summary: []repository.DailyAccuracy{{ModelKey: "ml"}},
		daily:   []repository.DailyAccuracy{{ModelKey: "ml"}},
		preds:   []domain.MLPrediction{{ModelKey: "ml"}},
	}
	svc := NewService(nil, nil, backtestSvc, nil)

	out, err := svc.GetBacktest(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Summary) != 1 || len(out.Daily) != 1 || len(out.Predictions) != 1 {
		t.Fatalf("expected backtest payload")
	}
	if backtestSvc.days != 30 || backtestSvc.limit != 50 {
		t.Fatalf("expected defaults days=30 limit=50, got days=%d limit=%d", backtestSvc.days, backtestSvc.limit)
	}
}
