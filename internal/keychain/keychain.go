// Package keychain wraps the OS keychain for credential storage.
// All secrets (client_secret, refresh_token, access_token) MUST be stored
// here and nowhere else — never in .env, disk files, or logs.
package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "tastytrade-cli"

// Key constants — centralised to prevent typo drift across callers.
const (
	KeyClientSecret  = "client_secret"
	KeyRefreshToken  = "refresh_token"
	KeyAccessToken   = "access_token"
	KeyTokenType     = "token_type"
)

// Set stores a value in the OS keychain under the given key.
func Set(key, value string) error {
	if err := keyring.Set(service, key, value); err != nil {
		return fmt.Errorf("keychain set %q: %w", key, err)
	}
	return nil
}

// Get retrieves a value from the OS keychain.
// Returns ("", ErrNotFound) if the key does not exist.
func Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if err != nil {
		return "", fmt.Errorf("keychain get %q: %w", key, err)
	}
	return v, nil
}

// Delete removes a key from the OS keychain. Idempotent.
func Delete(key string) error {
	err := keyring.Delete(service, key)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("keychain delete %q: %w", key, err)
	}
	return nil
}

// MustGet retrieves a value or returns an error with a human-readable hint.
func MustGet(key string) (string, error) {
	v, err := Get(key)
	if err != nil {
		return "", fmt.Errorf("%w\nhint: run 'tt login' to store credentials", err)
	}
	if v == "" {
		return "", fmt.Errorf("keychain key %q is empty — run 'tt login'", key)
	}
	return v, nil
}
