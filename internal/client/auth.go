package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/keychain"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"go.uber.org/zap"
)

// tokenState holds the in-memory token after a successful refresh.
// The source of truth for refresh_token is always the OS keychain.
type tokenState struct {
	mu           sync.RWMutex
	accessToken  string
	tokenType    string // read dynamically from /oauth/token — never hardcoded
	issuedAt     time.Time
}

func (t *tokenState) authHeader() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tokenType + " " + t.accessToken
}

func (t *tokenState) needsRefresh() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return time.Since(t.issuedAt) >= config.RefreshThreshold
}

// EnsureToken refreshes the access token if within the proactive refresh window.
// Safe to call concurrently — refresh is serialised under the write lock.
func (c *Client) EnsureToken(ctx context.Context) error {
	if !c.token.needsRefresh() {
		return nil
	}
	c.token.mu.Lock()
	defer c.token.mu.Unlock()
	// Double-check after acquiring write lock.
	if time.Since(c.token.issuedAt) < config.RefreshThreshold {
		return nil
	}
	return c.doTokenRefresh(ctx)
}

// doTokenRefresh exchanges the stored refresh_token for a new access_token.
// Must be called with c.token.mu held for writing.
func (c *Client) doTokenRefresh(ctx context.Context) error {
	refreshToken, err := keychain.MustGet(keychain.KeyRefreshToken)
	if err != nil {
		return fmt.Errorf("token refresh: cannot load refresh_token from keychain: %w", err)
	}
	clientSecret, err := keychain.MustGet(keychain.KeyClientSecret)
	if err != nil {
		return fmt.Errorf("token refresh: cannot load client_secret from keychain: %w", err)
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     c.cfg.ClientID,
		"client_secret": clientSecret,
		"refresh_token": refreshToken,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.BaseURL+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("token refresh: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	// Do NOT send Accept-Version on auth endpoints.

	resp, err := c.httpClient.Do(req)
	if err != nil {
		Metrics.TokenRefreshes.WithLabelValues("fail").Inc()
		return fmt.Errorf("token refresh: HTTP: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		Metrics.TokenRefreshes.WithLabelValues("fail").Inc()
		return fmt.Errorf("token refresh: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.TokenResponse]
	if err := json.Unmarshal(data, &env); err != nil {
		Metrics.TokenRefreshes.WithLabelValues("fail").Inc()
		return fmt.Errorf("token refresh: unmarshal: %w", err)
	}

	tok := env.Data
	c.token.accessToken = tok.AccessToken
	c.token.tokenType   = tok.TokenType
	c.token.issuedAt    = time.Now()

	// SAFE: only persist new refresh_token if non-empty.
	// If the response omits it (network truncation, transient error) we retain
	// the existing keychain value and log a warning. Do NOT overwrite with "".
	if tok.RefreshToken != "" {
		if err := keychain.Set(keychain.KeyRefreshToken, tok.RefreshToken); err != nil {
			c.log.Warn("keychain write failed — token in memory only",
				zap.String("key", keychain.KeyRefreshToken),
				zap.Error(err))
		}
		Metrics.TokenRefreshes.WithLabelValues("ok").Inc()
	} else {
		c.log.Warn("refresh_token absent from /oauth/token response — retaining existing keychain token",
			zap.String("action", "no-overwrite"))
		Metrics.TokenRefreshes.WithLabelValues("missing_refresh_token").Inc()
	}

	return nil
}
