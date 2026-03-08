package webconsole

import "time"

const (
	ClientTypeUICommand = "ui.command"
	ClientTypeUIPing    = "ui.ping"
	ClientTypeUIRefresh = "ui.refresh"
)

const (
	EventTypeUIStatus    = "ui.status"
	EventTypeUIChatReply = "ui.chat.reply"
	EventTypeUIError     = "ui.error"
	EventTypeUIHeartbeat = "ui.heartbeat"
)

type ClientMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Command   string `json:"command,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Event struct {
	Type      string    `json:"type"`
	SessionID string    `json:"session_id,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Seq       int64     `json:"seq,omitempty"`
	State     string    `json:"state,omitempty"`
	Code      string    `json:"code,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type SessionMeta struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	LastSeq   int64     `json:"last_seq"`
	TTL       int64     `json:"ttl_seconds"`
}

type SessionResponse struct {
	Session SessionMeta `json:"session"`
	Events  []Event     `json:"events,omitempty"`
	History []string    `json:"history,omitempty"`
}
