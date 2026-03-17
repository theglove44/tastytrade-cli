package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/keychain"
	"go.uber.org/zap"
)

func TestDoTokenRefresh_MissingRefreshTokenPreservesStoredToken(t *testing.T) {
	origMustGet := authKeychainMustGet
	origSet := authKeychainSet
	defer func() {
		authKeychainMustGet = origMustGet
		authKeychainSet = origSet
	}()

	stored := map[string]string{
		keychain.KeyRefreshToken: "existing-refresh",
		keychain.KeyClientSecret: "client-secret",
	}
	authKeychainMustGet = func(key string) (string, error) {
		return stored[key], nil
	}
	setCalls := 0
	authKeychainSet = func(key, value string) error {
		setCalls++
		stored[key] = value
		return nil
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","token_type":"Bearer","expires_in":900}`))
	}))
	defer ts.Close()

	log := zap.NewNop()
	c := &Client{
		cfg:        &config.Config{BaseURL: ts.URL, UserAgent: "test"},
		httpClient: ts.Client(),
		token:      &tokenState{},
		log:        log,
	}

	if err := c.doTokenRefresh(context.Background()); err != nil {
		t.Fatalf("doTokenRefresh: %v", err)
	}
	if got := stored[keychain.KeyRefreshToken]; got != "existing-refresh" {
		t.Fatalf("refresh token changed: got %q, want existing-refresh", got)
	}
	if c.token.accessToken != "new-access" {
		t.Fatalf("access token: got %q", c.token.accessToken)
	}
	if setCalls != 0 {
		t.Fatalf("refresh token should not be rewritten when omitted; setCalls=%d", setCalls)
	}
}

func TestDoTokenRefresh_NewRefreshTokenRotatesStoredToken(t *testing.T) {
	origMustGet := authKeychainMustGet
	origSet := authKeychainSet
	defer func() {
		authKeychainMustGet = origMustGet
		authKeychainSet = origSet
	}()

	stored := map[string]string{
		keychain.KeyRefreshToken: "existing-refresh",
		keychain.KeyClientSecret: "client-secret",
	}
	authKeychainMustGet = func(key string) (string, error) {
		return stored[key], nil
	}
	authKeychainSet = func(key, value string) error {
		stored[key] = value
		return nil
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access","token_type":"Bearer","refresh_token":"rotated-refresh","expires_in":900}`))
	}))
	defer ts.Close()

	c := &Client{
		cfg:        &config.Config{BaseURL: ts.URL, UserAgent: "test"},
		httpClient: ts.Client(),
		token:      &tokenState{},
		log:        zap.NewNop(),
	}

	if err := c.doTokenRefresh(context.Background()); err != nil {
		t.Fatalf("doTokenRefresh: %v", err)
	}
	if got := stored[keychain.KeyRefreshToken]; got != "rotated-refresh" {
		t.Fatalf("refresh token: got %q, want rotated-refresh", got)
	}
	if c.token.accessToken != "new-access" {
		t.Fatalf("access token: got %q", c.token.accessToken)
	}
	if c.token.issuedAt.IsZero() || time.Since(c.token.issuedAt) > time.Minute {
		t.Fatalf("issuedAt not updated: %v", c.token.issuedAt)
	}
}
