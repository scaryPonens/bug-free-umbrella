package mcp

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultMCPMaxBodyBytes int64 = 1 << 20 // 1MiB

type HTTPHandlerConfig struct {
	AuthToken       string
	RateLimitPerMin int
	MaxBodyBytes    int64
}

func wrapHTTPHandler(base http.Handler, cfg HTTPHandlerConfig) http.Handler {
	h := withBodyLimit(base, cfg.MaxBodyBytes)
	h = withRateLimit(h, newHTTPRateLimiter(cfg.RateLimitPerMin))
	h = withBearerAuth(h, cfg.AuthToken)
	return h
}

func withBearerAuth(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authz, "Bearer ") {
			writeJSONError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if token == "" || provided == "" || provided != token {
			writeJSONError(w, http.StatusForbidden, "invalid bearer token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withBodyLimit(next http.Handler, limit int64) http.Handler {
	if limit <= 0 {
		limit = defaultMCPMaxBodyBytes
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func withRateLimit(next http.Handler, limiter *httpRateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limiter == nil {
			next.ServeHTTP(w, r)
			return
		}
		if !limiter.Allow(rateLimitKey(r)) {
			writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func rateLimitKey(r *http.Request) string {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		host = "unknown"
	}
	if token == "" {
		return host
	}
	return token + "|" + host
}

type httpRateLimiter struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	bucket map[string]*tokenBucket
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newHTTPRateLimiter(perMin int) *httpRateLimiter {
	if perMin <= 0 {
		perMin = 60
	}
	return &httpRateLimiter{
		rate:   float64(perMin) / 60.0,
		burst:  float64(perMin),
		bucket: make(map[string]*tokenBucket),
	}
}

func (l *httpRateLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	if key == "" {
		key = "default"
	}

	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.bucket[key]
	if !ok {
		l.bucket[key] = &tokenBucket{tokens: l.burst - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
