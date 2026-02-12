package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPAuthMiddlewareRejectsMissingOrBadToken(t *testing.T) {
	h := wrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), HTTPHandlerConfig{AuthToken: "secret", RateLimitPerMin: 60, MaxBodyBytes: 1024})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "http://example.com/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHTTPAuthMiddlewareAllowsToolCalls(t *testing.T) {
	called := false
	h := wrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), HTTPHandlerConfig{AuthToken: "secret", RateLimitPerMin: 60, MaxBodyBytes: 1 << 20})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Fatal("expected wrapped handler to be invoked")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := withRateLimit(next, newHTTPRateLimiter(1))

	req1 := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req1.RemoteAddr = "127.0.0.1:1234"
	req1.Header.Set("Authorization", "Bearer secret")
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", w1.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	req2.Header.Set("Authorization", "Bearer secret")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate-limited, got %d", w2.Code)
	}
}
