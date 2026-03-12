package cmd

import (
	"testing"

	"github.com/theglove44/tastytrade-cli/internal/keychain"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

func TestPersistLoginCredentials_UsesEnteredRefreshTokenWhenResponseOmitsIt(t *testing.T) {
	orig := loginKeychainSet
	defer func() { loginKeychainSet = orig }()

	stored := map[string]string{}
	loginKeychainSet = func(key, value string) error {
		stored[key] = value
		return nil
	}

	tok := models.TokenResponse{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		ExpiresIn:   900,
	}
	if err := persistLoginCredentials("client-id", "client-secret", "entered-refresh", tok); err != nil {
		t.Fatalf("persistLoginCredentials: %v", err)
	}

	if got := stored[keychain.KeyRefreshToken]; got != "entered-refresh" {
		t.Fatalf("refresh token: got %q, want entered refresh token", got)
	}
	if got := stored[keychain.KeyAccessToken]; got != "access-token" {
		t.Fatalf("access token: got %q", got)
	}
}
