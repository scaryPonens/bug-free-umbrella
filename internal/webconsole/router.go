//go:build legacy_webconsole_emulator
// +build legacy_webconsole_emulator

package webconsole

import (
	contextpkg "context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"bug-free-umbrella/internal/domain"
	"bug-free-umbrella/internal/repository"

	"go.opentelemetry.io/otel/trace"
)

type PriceReader interface {
	GetCurrentPrices(ctx contextpkg.Context) ([]*domain.PriceSnapshot, error)
	GetCurrentPrice(ctx contextpkg.Context, symbol string) (*domain.PriceSnapshot, error)
}

type SignalReader interface {
	ListSignals(ctx contextpkg.Context, filter domain.SignalFilter) ([]domain.Signal, error)
}

type BacktestReader interface {
	GetAccuracySummary(ctx contextpkg.Context) ([]repository.DailyAccuracy, error)
	GetDailyAccuracy(ctx contextpkg.Context, modelKey string, days int) ([]repository.DailyAccuracy, error)
	ListRecentPredictions(ctx contextpkg.Context, limit int) ([]domain.MLPrediction, error)
}

type AdvisorReader interface {
	Ask(ctx contextpkg.Context, chatID int64, message string) (string, error)
}

type CommandRouter struct {
	tracer   trace.Tracer
	prices   PriceReader
	signals  SignalReader
	backtest BacktestReader
	advisor  AdvisorReader
	sessions *SessionManager
}

func NewCommandRouter(
	tracer trace.Tracer,
	prices PriceReader,
	signals SignalReader,
	backtest BacktestReader,
	advisor AdvisorReader,
	sessions *SessionManager,
) *CommandRouter {
	return &CommandRouter{
		tracer:   tracer,
		prices:   prices,
		signals:  signals,
		backtest: backtest,
		advisor:  advisor,
		sessions: sessions,
	}
}

func (r *CommandRouter) Execute(ctx contextpkg.Context, sessionID, requestID, line string, emit func(Event) error) error {
	if r.sessions != nil {
		_ = r.sessions.PushHistory(ctx, sessionID, line)
	}

	parsed, err := ParseCommand(line)
	if err != nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "INVALID_COMMAND", Message: err.Error()})
	}

	switch parsed.Name {
	case "help":
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "ansi", Chunk: helpText()})
	case "clear":
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "control", Chunk: "clear"})
	case "history":
		return r.execHistory(ctx, sessionID, requestID, emit)
	case "status":
		return r.execStatus(requestID, emit)
	case "prices":
		return r.execPrices(ctx, parsed, requestID, emit)
	case "signals":
		return r.execSignals(ctx, parsed, requestID, emit)
	case "dashboard":
		if err := r.execPrices(ctx, ParsedCommand{Name: "prices", Flags: map[string]string{}}, requestID, emit); err != nil {
			return err
		}
		return r.execSignals(ctx, ParsedCommand{Name: "signals", Flags: map[string]string{"limit": "10"}}, requestID, emit)
	case "backtest":
		return r.execBacktest(ctx, parsed, requestID, emit)
	case "ask":
		return r.execAsk(ctx, sessionID, parsed, requestID, emit)
	default:
		return emit(Event{
			Type:      EventTypeError,
			RequestID: requestID,
			Code:      "UNSUPPORTED_COMMAND",
			Message:   fmt.Sprintf("unsupported command: %s", parsed.Name),
		})
	}
}

func (r *CommandRouter) execHistory(ctx contextpkg.Context, sessionID, requestID string, emit func(Event) error) error {
	if r.sessions == nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "HISTORY_UNAVAILABLE", Message: "history unavailable"})
	}
	items, err := r.sessions.ListHistory(ctx, sessionID, 50)
	if err != nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "HISTORY_ERROR", Message: err.Error()})
	}
	if len(items) == 0 {
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no history"})
	}
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%2d  %s\n", i+1, item))
	}
	return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.TrimRight(sb.String(), "\n")})
}

func (r *CommandRouter) execStatus(requestID string, emit func(Event) error) error {
	status := []string{
		fmt.Sprintf("time: %s", time.Now().UTC().Format(time.RFC3339)),
		fmt.Sprintf("prices: %t", r.prices != nil),
		fmt.Sprintf("signals: %t", r.signals != nil),
		fmt.Sprintf("backtest: %t", r.backtest != nil),
		fmt.Sprintf("advisor: %t", r.advisor != nil),
	}
	return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(status, "\n")})
}

func (r *CommandRouter) execPrices(ctx contextpkg.Context, parsed ParsedCommand, requestID string, emit func(Event) error) error {
	if r.prices == nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "SERVICE_UNAVAILABLE", Message: "price service unavailable"})
	}
	symbol := strings.ToUpper(strings.TrimSpace(parsed.Flags["symbol"]))
	if symbol != "" {
		snapshot, err := r.prices.GetCurrentPrice(ctx, symbol)
		if err != nil {
			return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "PRICE_ERROR", Message: err.Error()})
		}
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: formatPrice(snapshot)})
	}

	prices, err := r.prices.GetCurrentPrices(ctx)
	if err != nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "PRICE_ERROR", Message: err.Error()})
	}
	if len(prices) == 0 {
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no prices available"})
	}
	var lines []string
	for _, price := range prices {
		lines = append(lines, formatPrice(price))
	}
	return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(lines, "\n")})
}

func (r *CommandRouter) execSignals(ctx contextpkg.Context, parsed ParsedCommand, requestID string, emit func(Event) error) error {
	if r.signals == nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "SERVICE_UNAVAILABLE", Message: "signal service unavailable"})
	}
	limit := 20
	if raw := strings.TrimSpace(parsed.Flags["limit"]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	filter := domain.SignalFilter{Limit: limit}
	if symbol := strings.TrimSpace(parsed.Flags["symbol"]); symbol != "" {
		filter.Symbol = strings.ToUpper(symbol)
	}
	if indicator := strings.TrimSpace(parsed.Flags["indicator"]); indicator != "" {
		filter.Indicator = strings.ToLower(indicator)
	}
	if raw := strings.TrimSpace(parsed.Flags["risk"]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			risk := domain.RiskLevel(n)
			filter.Risk = &risk
		}
	}
	items, err := r.signals.ListSignals(ctx, filter)
	if err != nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "SIGNAL_ERROR", Message: err.Error()})
	}
	if len(items) == 0 {
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no signals found"})
	}
	var lines []string
	for _, signal := range items {
		lines = append(lines, formatSignal(signal))
	}
	return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(lines, "\n")})
}

func (r *CommandRouter) execBacktest(ctx contextpkg.Context, parsed ParsedCommand, requestID string, emit func(Event) error) error {
	if r.backtest == nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "SERVICE_UNAVAILABLE", Message: "backtest unavailable"})
	}
	view := strings.ToLower(strings.TrimSpace(parsed.Flags["view"]))
	if view == "" {
		view = "summary"
	}
	switch view {
	case "summary":
		summary, err := r.backtest.GetAccuracySummary(ctx)
		if err != nil {
			return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "BACKTEST_ERROR", Message: err.Error()})
		}
		if len(summary) == 0 {
			return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no backtest data"})
		}
		var lines []string
		for _, item := range summary {
			lines = append(lines, fmt.Sprintf("%s accuracy=%.1f%% total=%d correct=%d", item.ModelKey, item.Accuracy*100, item.Total, item.Correct))
		}
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(lines, "\n")})
	case "daily":
		days := 30
		if raw := strings.TrimSpace(parsed.Flags["days"]); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				days = n
			}
		}
		model := strings.TrimSpace(parsed.Flags["model"])
		daily, err := r.backtest.GetDailyAccuracy(ctx, model, days)
		if err != nil {
			return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "BACKTEST_ERROR", Message: err.Error()})
		}
		if len(daily) == 0 {
			return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no daily backtest data"})
		}
		var lines []string
		for _, item := range daily {
			lines = append(lines, fmt.Sprintf("%s %s accuracy=%.1f%% %d/%d", item.DayUTC.Format("2006-01-02"), item.ModelKey, item.Accuracy*100, item.Correct, item.Total))
		}
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(lines, "\n")})
	case "predictions":
		limit := 20
		if raw := strings.TrimSpace(parsed.Flags["limit"]); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		preds, err := r.backtest.ListRecentPredictions(ctx, limit)
		if err != nil {
			return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "BACKTEST_ERROR", Message: err.Error()})
		}
		if len(preds) == 0 {
			return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: "no predictions"})
		}
		var lines []string
		for _, pred := range preds {
			correct := "n/a"
			if pred.IsCorrect != nil {
				correct = strconv.FormatBool(*pred.IsCorrect)
			}
			lines = append(lines, fmt.Sprintf("%s %s %s dir=%s risk=%d correct=%s", pred.Symbol, pred.Interval, pred.ModelKey, pred.Direction, pred.Risk, correct))
		}
		return emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: strings.Join(lines, "\n")})
	default:
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "INVALID_VIEW", Message: "backtest view must be summary|daily|predictions"})
	}
}

func (r *CommandRouter) execAsk(ctx contextpkg.Context, sessionID string, parsed ParsedCommand, requestID string, emit func(Event) error) error {
	if r.advisor == nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "SERVICE_UNAVAILABLE", Message: "advisor unavailable"})
	}
	question := strings.Join(parsed.Position, " ")
	if question == "" {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "INVALID_COMMAND", Message: "usage: ask <question>"})
	}
	chatID := chatIDFromSession(sessionID)
	reply, err := r.advisor.Ask(ctx, chatID, question)
	if err != nil {
		return emit(Event{Type: EventTypeError, RequestID: requestID, Code: "ADVISOR_ERROR", Message: err.Error()})
	}

	for _, chunk := range splitChunks(reply, 64) {
		if err := emit(Event{Type: EventTypeCommandOutput, RequestID: requestID, Stream: "stdout", Format: "plain", Chunk: chunk}); err != nil {
			return err
		}
	}
	return nil
}

func helpText() string {
	return strings.Join([]string{
		"available commands:",
		"  help",
		"  clear",
		"  history",
		"  status",
		"  prices [--symbol BTC]",
		"  signals [--symbol BTC] [--risk 1..5] [--indicator rsi] [--limit N]",
		"  dashboard",
		"  backtest [--view summary|daily|predictions] [--days N] [--model key] [--limit N]",
		"  ask <question>",
	}, "\n")
}

func formatPrice(price *domain.PriceSnapshot) string {
	if price == nil {
		return "n/a"
	}
	sign := ""
	if price.Change24hPct > 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s price=$%.4f change24h=%s%.2f%% volume24h=$%.0f", price.Symbol, price.PriceUSD, sign, price.Change24hPct, price.Volume24h)
}

func formatSignal(signal domain.Signal) string {
	return fmt.Sprintf("#%d %s %s %s %s risk=%d %s",
		signal.ID,
		signal.Symbol,
		signal.Interval,
		signal.Indicator,
		signal.Direction,
		signal.Risk,
		signal.Timestamp.UTC().Format(time.RFC3339),
	)
}

func chatIDFromSession(sessionID string) int64 {
	var out int64
	for i := 0; i < len(sessionID); i++ {
		out += int64(sessionID[i])
	}
	return -(out + 2_000_000)
}

func splitChunks(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var chunks []string
	runes := []rune(text)
	for len(runes) > width {
		chunks = append(chunks, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}
