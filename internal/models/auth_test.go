package models_test

import (
	"encoding/json"
	"testing"

	"github.com/theglove44/tastytrade-cli/internal/models"
)

// TestTokenResponse_FlatUnderscoreKeys is the canonical regression test for the
// auth parsing bug. The /oauth/token endpoint returns a flat RFC 6749 response
// with underscore-separated keys — not dashes, not a DataEnvelope wrapper.
//
// If this test fails, the JSON tags on TokenResponse are wrong.
func TestTokenResponse_FlatUnderscoreKeys(t *testing.T) {
	// This is the exact shape returned by TastyTrade /oauth/token,
	// confirmed against the Python SDK (tastytrade-sdk-python).
	raw := `{
		"access_token":  "eyJhbGciOiJSUzI1NiJ9.access",
		"token_type":    "Bearer",
		"refresh_token": "eyJhbGciOiJSUzI1NiJ9.refresh",
		"expires_in":    900
	}`

	var tok models.TokenResponse
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if tok.AccessToken != "eyJhbGciOiJSUzI1NiJ9.access" {
		t.Errorf("AccessToken: got %q — underscore tag 'access_token' not working", tok.AccessToken)
	}
	if tok.TokenType != "Bearer" {
		t.Errorf("TokenType: got %q", tok.TokenType)
	}
	if tok.RefreshToken != "eyJhbGciOiJSUzI1NiJ9.refresh" {
		t.Errorf("RefreshToken: got %q", tok.RefreshToken)
	}
	if tok.ExpiresIn != 900 {
		t.Errorf("ExpiresIn: got %d, want 900", tok.ExpiresIn)
	}
}

// TestTokenResponse_DashedKeys_DoNotParse verifies that the OLD (broken) dashed
// format does NOT populate the struct. This ensures we cannot accidentally
// revert to parsing dashes.
func TestTokenResponse_DashedKeys_DoNotParse(t *testing.T) {
	// Old format (broken): dashed keys, DataEnvelope wrapper — must NOT parse.
	raw := `{
		"access-token":  "should-not-parse",
		"token-type":    "Bearer",
		"refresh-token": "should-not-parse",
		"expires-in":    900
	}`

	var tok models.TokenResponse
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// All fields must be zero-value — dashed keys should not match underscore tags.
	if tok.AccessToken != "" {
		t.Errorf("AccessToken must be empty for dashed keys, got %q — JSON tags have reverted to dashes", tok.AccessToken)
	}
	if tok.RefreshToken != "" {
		t.Errorf("RefreshToken must be empty for dashed keys, got %q", tok.RefreshToken)
	}
}

// TestTokenResponse_NotWrappedInDataEnvelope verifies that attempting to parse
// a DataEnvelope-wrapped response into TokenResponse directly yields empty
// fields — reinforcing that the flat path is the only correct one.
func TestTokenResponse_NotWrappedInDataEnvelope(t *testing.T) {
	// This is what the OLD code tried to parse via DataEnvelope[TokenResponse].
	// Direct unmarshal into TokenResponse must yield empty fields.
	wrapped := `{"data": {"access_token": "tok", "token_type": "Bearer", "expires_in": 900}}`

	var tok models.TokenResponse
	if err := json.Unmarshal([]byte(wrapped), &tok); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	// Flat unmarshal of a wrapped response gives empty AccessToken —
	// the caller must NOT use DataEnvelope for /oauth/token.
	if tok.AccessToken != "" {
		t.Errorf("Direct unmarshal of DataEnvelope should yield empty AccessToken, got %q", tok.AccessToken)
	}
}

// TestTokenResponse_MissingRefreshToken verifies the empty-refresh-token path
// is handled safely — callers must check and not overwrite with empty string.
func TestTokenResponse_MissingRefreshToken(t *testing.T) {
	raw := `{"access_token": "eyJ.access", "token_type": "Bearer", "expires_in": 900}`

	var tok models.TokenResponse
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if tok.AccessToken == "" {
		t.Error("AccessToken should be populated")
	}
	if tok.RefreshToken != "" {
		t.Errorf("RefreshToken should be empty when absent from response, got %q", tok.RefreshToken)
	}
}
