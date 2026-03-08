package webconsole

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultAuthTTL   = 24 * time.Hour
	authCookieName   = "webconsole_auth"
	authCookiePath   = "/"
	authCookieMaxAge = 86400
)

type AuthService struct {
	redis *redis.Client
	ttl   time.Duration
	key   []byte
}

func NewAuthService(redisClient *redis.Client, ttl time.Duration, secret string) *AuthService {
	if ttl <= 0 {
		ttl = defaultAuthTTL
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		secret = "web-console-dev-secret"
	}
	return &AuthService{redis: redisClient, ttl: ttl, key: []byte(secret)}
}

func (a *AuthService) CookieName() string {
	return authCookieName
}

func (a *AuthService) Login(expectedAPIKey, providedAPIKey string) (string, error) {
	expectedAPIKey = strings.TrimSpace(expectedAPIKey)
	providedAPIKey = strings.TrimSpace(providedAPIKey)
	if expectedAPIKey != "" && providedAPIKey != expectedAPIKey {
		return "", fmt.Errorf("invalid API key")
	}
	raw, err := randomToken(24)
	if err != nil {
		return "", err
	}
	sig := a.sign(raw)
	return raw + "." + sig, nil
}

func (a *AuthService) SaveToken(ctx context.Context, cookieValue string) error {
	if a.redis == nil {
		return fmt.Errorf("redis client is required")
	}
	raw, ok := a.verify(cookieValue)
	if !ok {
		return fmt.Errorf("invalid cookie token")
	}
	return a.redis.Set(ctx, a.authKey(raw), "operator", a.ttl).Err()
}

func (a *AuthService) Validate(ctx context.Context, cookieValue string) error {
	if a.redis == nil {
		return fmt.Errorf("redis client is required")
	}
	raw, ok := a.verify(cookieValue)
	if !ok {
		return fmt.Errorf("invalid cookie token")
	}
	exists, err := a.redis.Exists(ctx, a.authKey(raw)).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("session expired")
	}
	if err := a.redis.Expire(ctx, a.authKey(raw), a.ttl).Err(); err != nil {
		return err
	}
	return nil
}

func (a *AuthService) Logout(ctx context.Context, cookieValue string) error {
	if a.redis == nil {
		return nil
	}
	raw, ok := a.verify(cookieValue)
	if !ok {
		return nil
	}
	return a.redis.Del(ctx, a.authKey(raw)).Err()
}

func (a *AuthService) authKey(raw string) string {
	return "webconsole:auth:" + raw
}

func (a *AuthService) sign(raw string) string {
	h := hmac.New(sha256.New, a.key)
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

func (a *AuthService) verify(cookieValue string) (string, bool) {
	parts := strings.Split(cookieValue, ".")
	if len(parts) != 2 {
		return "", false
	}
	raw := strings.TrimSpace(parts[0])
	sig := strings.TrimSpace(parts[1])
	if raw == "" || sig == "" {
		return "", false
	}
	expected := a.sign(raw)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return "", false
	}
	return raw, true
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
