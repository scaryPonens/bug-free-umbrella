package webconsole

import (
	contextpkg "context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultSessionTTL   = 24 * time.Hour
	defaultEventLimit   = 200
	defaultHistoryLimit = 50
	maxStoredEvents     = 4000
	maxStoredHistory    = 200
)

type SessionManager struct {
	redis *redis.Client
	ttl   time.Duration
}

func NewSessionManager(redisClient *redis.Client, ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &SessionManager{redis: redisClient, ttl: ttl}
}

func (m *SessionManager) CreateSession(ctx contextpkg.Context) (SessionMeta, error) {
	if m.redis == nil {
		return SessionMeta{}, fmt.Errorf("redis client is required")
	}

	now := time.Now().UTC()
	sessionID, err := randomID(16)
	if err != nil {
		return SessionMeta{}, err
	}

	meta := SessionMeta{
		SessionID: sessionID,
		CreatedAt: now,
		LastSeen:  now,
		LastSeq:   0,
		TTL:       int64(m.ttl.Seconds()),
	}

	if err := m.writeMeta(ctx, meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (m *SessionManager) EnsureSession(ctx contextpkg.Context, sessionID string) (SessionMeta, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return m.CreateSession(ctx)
	}
	meta, err := m.GetSession(ctx, sessionID)
	if err == redis.Nil {
		return SessionMeta{}, fmt.Errorf("session not found")
	}
	if err != nil {
		return SessionMeta{}, err
	}
	meta.LastSeen = time.Now().UTC()
	if err := m.writeMeta(ctx, meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (m *SessionManager) GetSession(ctx contextpkg.Context, sessionID string) (SessionMeta, error) {
	if m.redis == nil {
		return SessionMeta{}, fmt.Errorf("redis client is required")
	}
	values, err := m.redis.HGetAll(ctx, m.metaKey(sessionID)).Result()
	if err != nil {
		return SessionMeta{}, err
	}
	if len(values) == 0 {
		return SessionMeta{}, redis.Nil
	}

	createdAt, err := parseUnix(values["created_at"])
	if err != nil {
		return SessionMeta{}, err
	}
	lastSeen, err := parseUnix(values["last_seen"])
	if err != nil {
		return SessionMeta{}, err
	}
	lastSeq, err := strconv.ParseInt(values["last_seq"], 10, 64)
	if err != nil {
		lastSeq = 0
	}

	return SessionMeta{
		SessionID: sessionID,
		CreatedAt: createdAt,
		LastSeen:  lastSeen,
		LastSeq:   lastSeq,
		TTL:       int64(m.ttl.Seconds()),
	}, nil
}

func (m *SessionManager) AppendEvent(ctx contextpkg.Context, sessionID string, event Event) (Event, error) {
	if m.redis == nil {
		return Event{}, fmt.Errorf("redis client is required")
	}
	seq, err := m.redis.Incr(ctx, m.seqKey(sessionID)).Result()
	if err != nil {
		return Event{}, err
	}

	event.Seq = seq
	event.SessionID = sessionID
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}

	pipe := m.redis.TxPipeline()
	pipe.ZAdd(ctx, m.eventsKey(sessionID), redis.Z{Score: float64(seq), Member: payload})
	pipe.ZRemRangeByRank(ctx, m.eventsKey(sessionID), 0, -maxStoredEvents-1)
	pipe.HSet(ctx, m.metaKey(sessionID), "last_seq", strconv.FormatInt(seq, 10), "last_seen", strconv.FormatInt(time.Now().UTC().Unix(), 10))
	pipe.Expire(ctx, m.metaKey(sessionID), m.ttl)
	pipe.Expire(ctx, m.seqKey(sessionID), m.ttl)
	pipe.Expire(ctx, m.eventsKey(sessionID), m.ttl)
	pipe.Expire(ctx, m.historyKey(sessionID), m.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (m *SessionManager) ListEventsSince(ctx contextpkg.Context, sessionID string, lastSeq int64, limit int64) ([]Event, error) {
	if m.redis == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if limit <= 0 || limit > 1000 {
		limit = defaultEventLimit
	}
	members, err := m.redis.ZRangeByScore(ctx, m.eventsKey(sessionID), &redis.ZRangeBy{
		Min:   fmt.Sprintf("(%d", lastSeq),
		Max:   "+inf",
		Count: limit,
	}).Result()
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(members))
	for _, member := range members {
		var event Event
		if err := json.Unmarshal([]byte(member), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func (m *SessionManager) PushHistory(ctx contextpkg.Context, sessionID, line string) error {
	if m.redis == nil {
		return fmt.Errorf("redis client is required")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	pipe := m.redis.TxPipeline()
	pipe.LPush(ctx, m.historyKey(sessionID), line)
	pipe.LTrim(ctx, m.historyKey(sessionID), 0, maxStoredHistory-1)
	pipe.Expire(ctx, m.historyKey(sessionID), m.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *SessionManager) ListHistory(ctx contextpkg.Context, sessionID string, limit int64) ([]string, error) {
	if m.redis == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if limit <= 0 || limit > 200 {
		limit = defaultHistoryLimit
	}
	items, err := m.redis.LRange(ctx, m.historyKey(sessionID), 0, limit-1).Result()
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func (m *SessionManager) writeMeta(ctx contextpkg.Context, meta SessionMeta) error {
	pipe := m.redis.TxPipeline()
	pipe.HSet(ctx, m.metaKey(meta.SessionID),
		"created_at", strconv.FormatInt(meta.CreatedAt.UTC().Unix(), 10),
		"last_seen", strconv.FormatInt(meta.LastSeen.UTC().Unix(), 10),
		"last_seq", strconv.FormatInt(meta.LastSeq, 10),
	)
	pipe.SetNX(ctx, m.seqKey(meta.SessionID), meta.LastSeq, m.ttl)
	pipe.Expire(ctx, m.metaKey(meta.SessionID), m.ttl)
	pipe.Expire(ctx, m.seqKey(meta.SessionID), m.ttl)
	pipe.Expire(ctx, m.eventsKey(meta.SessionID), m.ttl)
	pipe.Expire(ctx, m.historyKey(meta.SessionID), m.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (m *SessionManager) metaKey(sessionID string) string {
	return "webconsole:session:" + sessionID + ":meta"
}

func (m *SessionManager) seqKey(sessionID string) string {
	return "webconsole:session:" + sessionID + ":seq"
}

func (m *SessionManager) eventsKey(sessionID string) string {
	return "webconsole:session:" + sessionID + ":events"
}

func (m *SessionManager) historyKey(sessionID string) string {
	return "webconsole:session:" + sessionID + ":history"
}

func parseUnix(raw string) (time.Time, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(v, 0).UTC(), nil
}

func randomID(numBytes int) (string, error) {
	if numBytes <= 0 {
		numBytes = 16
	}
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
