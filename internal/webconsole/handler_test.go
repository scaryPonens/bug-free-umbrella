package webconsole

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
)

type advisorTestStub struct {
	reply string
	err   error
}

func (s *advisorTestStub) Ask(ctx context.Context, chatID int64, message string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.reply, nil
}

func TestWebConsoleLoginAndSessionAuth(t *testing.T) {
	baseURL, _, _, shutdown := testWebConsoleServer(t, "expected", &advisorTestStub{reply: "ok"})
	defer shutdown()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/web-console/session", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("session request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d", res.StatusCode)
	}
}

func TestWebConsoleWebSocketAskFlow(t *testing.T) {
	baseURL, sessionID, cookieHeader, shutdown := testWebConsoleServer(t, "expected", &advisorTestStub{reply: "advisor reply"})
	defer shutdown()

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/web-console/ws?session_id=" + sessionID
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	header := http.Header{"Cookie": []string{cookieHeader}}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	defer conn.Close()

	first := readEvent(t, conn)
	if first.Type != EventTypeUIStatus || first.State != "connected" {
		t.Fatalf("expected connected status, got %#v", first)
	}

	if err := conn.WriteJSON(ClientMessage{
		Type:      ClientTypeUICommand,
		SessionID: sessionID,
		RequestID: "req-1",
		Command:   "ask",
		Message:   "What do you see?",
	}); err != nil {
		t.Fatalf("write message: %v", err)
	}

	thinking := waitForEvent(t, conn, func(e Event) bool {
		return e.Type == EventTypeUIStatus && e.RequestID == "req-1" && e.State == "thinking"
	})
	if thinking.Message == "" {
		t.Fatalf("expected thinking message")
	}

	reply := waitForEvent(t, conn, func(e Event) bool {
		return e.Type == EventTypeUIChatReply && e.RequestID == "req-1"
	})
	if reply.Message != "advisor reply" {
		t.Fatalf("expected advisor reply, got %q", reply.Message)
	}

	idle := waitForEvent(t, conn, func(e Event) bool {
		return e.Type == EventTypeUIStatus && e.RequestID == "req-1" && e.State == "idle"
	})
	if idle.Message == "" {
		t.Fatalf("expected idle message")
	}
}

func testWebConsoleServer(t *testing.T, apiKey string, advisor AdvisorReader) (baseURL string, sessionID string, cookieHeader string, shutdown func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	auth := NewAuthService(client, 24*time.Hour, "test-secret")
	sessions := NewSessionManager(client, 24*time.Hour)
	svc := NewService(nil, nil, nil, advisor)
	handler := NewHandler(otel.Tracer("test"), auth, sessions, svc, HandlerConfig{
		ExpectedAPIKey: apiKey,
		Heartbeat:      time.Hour,
	})

	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/web-console"))
	server := httptest.NewServer(router)

	loginReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/web-console/login", nil)
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	loginReq.Header.Set("X-API-Key", apiKey)
	loginRes, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRes.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(loginRes.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	_ = loginRes.Body.Close()

	sessionMap, ok := payload["session"].(map[string]any)
	if !ok {
		t.Fatalf("missing session payload")
	}
	rawID, ok := sessionMap["session_id"].(string)
	if !ok || rawID == "" {
		t.Fatalf("missing session_id")
	}

	var cookie *http.Cookie
	for _, c := range loginRes.Cookies() {
		if c.Name == auth.CookieName() {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatalf("missing auth cookie")
	}
	header := cookie.Name + "=" + cookie.Value

	return server.URL, rawID, header, func() {
		server.Close()
		_ = client.Close()
		mini.Close()
	}
}

func readEvent(t *testing.T, conn *websocket.Conn) Event {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var event Event
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("read event: %v", err)
	}
	return event
}

func waitForEvent(t *testing.T, conn *websocket.Conn, match func(Event) bool) Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := readEvent(t, conn)
		if match(event) {
			return event
		}
	}
	t.Fatalf("timed out waiting for event")
	return Event{}
}
