package webconsole

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const defaultHeartbeat = 20 * time.Second

type HandlerConfig struct {
	ExpectedAPIKey string
	Heartbeat      time.Duration
}

type Handler struct {
	tracer   trace.Tracer
	auth     *AuthService
	sessions *SessionManager
	service  *Service
	cfg      HandlerConfig
	upgrader websocket.Upgrader
}

func NewHandler(
	tracer trace.Tracer,
	auth *AuthService,
	sessions *SessionManager,
	service *Service,
	cfg HandlerConfig,
) *Handler {
	if cfg.Heartbeat <= 0 {
		cfg.Heartbeat = defaultHeartbeat
	}
	return &Handler{
		tracer:   tracer,
		auth:     auth,
		sessions: sessions,
		service:  service,
		cfg:      cfg,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.POST("/login", h.Login)
	r.POST("/logout", h.Logout)
	r.GET("/session", h.GetSession)
	r.GET("/ws", h.WebSocket)
}

func (h *Handler) Login(c *gin.Context) {
	ctx, span := h.tracer.Start(c.Request.Context(), "webconsole.login")
	defer span.End()

	provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
	cookieValue, err := h.auth.Login(h.cfg.ExpectedAPIKey, provided)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
		return
	}
	if err := h.auth.SaveToken(ctx, cookieValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.auth.CookieName(),
		Value:    cookieValue,
		Path:     authCookiePath,
		HttpOnly: true,
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   authCookieMaxAge,
	})

	meta, err := h.sessions.CreateSession(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "session": meta})
}

func (h *Handler) Logout(c *gin.Context) {
	cookie, err := c.Request.Cookie(h.auth.CookieName())
	if err == nil {
		_ = h.auth.Logout(c.Request.Context(), cookie.Value)
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.auth.CookieName(),
		Value:    "",
		Path:     authCookiePath,
		HttpOnly: true,
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) GetSession(c *gin.Context) {
	ctx, span := h.tracer.Start(c.Request.Context(), "webconsole.session")
	defer span.End()
	if err := h.requireAuth(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	meta, err := h.sessions.EnsureSession(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	since := parseInt64(c.Query("since"), 0)
	limit := parseInt64(c.Query("limit"), defaultEventLimit)
	events, err := h.sessions.ListEventsSince(ctx, meta.SessionID, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	history, err := h.sessions.ListHistory(ctx, meta.SessionID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, SessionResponse{Session: meta, Events: events, History: history})
}

func (h *Handler) WebSocket(c *gin.Context) {
	ctx, span := h.tracer.Start(c.Request.Context(), "webconsole.ws.connect")
	defer span.End()
	if err := h.requireAuth(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	sessionID := strings.TrimSpace(c.Query("session_id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	meta, err := h.sessions.EnsureSession(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	span.SetAttributes(attribute.String("session_id", meta.SessionID))

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("webconsole websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	client := newWSClient(conn)
	defer client.closeAll()

	if err := h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIStatus, State: "connected", Message: "websocket connected"}); err != nil {
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * h.cfg.Heartbeat))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(2 * h.cfg.Heartbeat))
	})

	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(h.cfg.Heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				if err := h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIHeartbeat, State: "alive"}); err != nil {
					return
				}
				if err := client.ping(); err != nil {
					return
				}
			}
		}
	}()
	defer close(heartbeatDone)

	for {
		var msg ClientMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return
		}
		switch strings.ToLower(strings.TrimSpace(msg.Type)) {
		case ClientTypeUIPing:
			_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIHeartbeat, RequestID: msg.RequestID, State: "alive"})
		case ClientTypeUIRefresh:
			_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIStatus, RequestID: msg.RequestID, State: "refreshed", Message: "refresh acknowledged"})
		case ClientTypeUICommand:
			requestID := strings.TrimSpace(msg.RequestID)
			if requestID == "" {
				requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
			}
			command := strings.ToLower(strings.TrimSpace(msg.Command))
			if command != "ask" {
				_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIError, RequestID: requestID, Code: "UNSUPPORTED_COMMAND", Message: "only ask command is supported"})
				continue
			}
			question := strings.TrimSpace(msg.Message)
			if question == "" {
				_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIError, RequestID: requestID, Code: "INVALID_COMMAND", Message: "message is required"})
				continue
			}
			_ = h.sessions.PushHistory(ctx, meta.SessionID, question)
			_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIStatus, RequestID: requestID, State: "thinking", Message: "advisor is thinking"})

			go func(reqID string, text string) {
				reply, err := h.service.Ask(ctx, meta.SessionID, text)
				if err != nil {
					_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIError, RequestID: reqID, Code: "ADVISOR_ERROR", Message: err.Error()})
					_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIStatus, RequestID: reqID, State: "idle", Message: "advisor error"})
					return
				}
				_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIChatReply, RequestID: reqID, State: "assistant", Message: reply})
				_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIStatus, RequestID: reqID, State: "idle", Message: "reply ready"})
			}(requestID, question)
		default:
			_ = h.emitEvent(context.Background(), client, meta.SessionID, Event{Type: EventTypeUIError, RequestID: msg.RequestID, Code: "UNKNOWN_EVENT", Message: "unsupported client message type"})
		}
	}
}

func (h *Handler) emitEvent(ctx context.Context, client *wsClient, sessionID string, event Event) error {
	persisted, err := h.sessions.AppendEvent(ctx, sessionID, event)
	if err != nil {
		return err
	}
	return client.send(persisted)
}

func (h *Handler) requireAuth(c *gin.Context) error {
	if h.auth == nil {
		return fmt.Errorf("auth unavailable")
	}
	cookie, err := c.Request.Cookie(h.auth.CookieName())
	if err != nil {
		return fmt.Errorf("missing auth cookie")
	}
	if err := h.auth.Validate(c.Request.Context(), cookie.Value); err != nil {
		return err
	}
	return nil
}

func parseInt64(raw string, fallback int64) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}
