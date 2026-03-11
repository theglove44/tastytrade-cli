package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/keychain"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Store OAuth credentials in the OS keychain",
	Long: `tt login stores your TastyTrade OAuth credentials in the OS keychain.

You will need:
  • Your OAuth client_id  (from developer.tastytrade.com)
  • Your OAuth client_secret
  • An initial refresh_token (generate via: developer portal > OAuth Applications > Manage > Create Grant)

Credentials are stored in the OS keychain under the service 'tastytrade-cli'.
They are never written to disk or environment variables.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin(cmd.Context())
	},
}

func runLogin(ctx context.Context) error {
	r := bufio.NewReader(os.Stdin)
	readLine := func(prompt string) (string, error) {
		fmt.Print(prompt)
		line, err := r.ReadString('\n')
		return strings.TrimSpace(line), err
	}
	readSecret := func(prompt string) (string, error) {
		fmt.Print(prompt)
		b, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println()
		return string(b), err
	}

	clientID, err := readLine("Client ID: ")
	if err != nil || clientID == "" {
		return fmt.Errorf("client_id is required")
	}

	clientSecret, err := readSecret("Client Secret: ")
	if err != nil || clientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}

	refreshToken, err := readSecret("Refresh Token (from developer portal Create Grant): ")
	if err != nil || refreshToken == "" {
		return fmt.Errorf("refresh_token is required")
	}

	// Build a minimal config sufficient for the bootstrap client.
	// We do not call config.Load() here because TASTYTRADE_CLIENT_ID is not yet
	// stored — the user is providing it interactively right now.
	baseURL := os.Getenv("TASTYTRADE_BASE_URL")
	if baseURL == "" {
		baseURL = config.SandboxBaseURL
	}
	userAgent := os.Getenv("TASTYTRADE_USER_AGENT")
	if userAgent == "" {
		userAgent = "tastytrade-cli/1.0.0"
	}
	bootstrapCfg := &config.Config{
		BaseURL:    baseURL,
		ClientID:   clientID,
		UserAgent:  userAgent,
		RateLimits: config.DefaultRateLimits(),
	}

	// Use the unauthenticated bootstrap client so login flows through the same
	// middleware stack as every other command: rate limiting, structured logging,
	// retry, Retry-After, metrics, X-Request-ID.
	// Authorization header is omitted by this constructor.
	bootClient := client.NewUnauthenticated(bootstrapCfg, logger)

	fmt.Printf("\nValidating credentials against %s ...\n", baseURL)

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     clientID,
		"client_secret": clientSecret,
		"refresh_token": refreshToken,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Accept-Version suppressed via SkipVersion — auth endpoints must not receive it.

	resp, err := bootClient.Do(ctx, req, client.FamilyAuth, client.RequestOptions{SkipVersion: true})
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.TokenResponse]
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}
	tok := env.Data
	if tok.AccessToken == "" {
		return fmt.Errorf("token exchange succeeded but access_token is empty — unexpected response")
	}

	// Persist credentials to OS keychain individually so each can be guarded.
	for key, value := range map[string]string{
		keychain.KeyClientSecret: clientSecret,
		keychain.KeyAccessToken:  tok.AccessToken,
		keychain.KeyTokenType:    tok.TokenType,
		"client_id":              clientID,
	} {
		if err := keychain.Set(key, value); err != nil {
			return fmt.Errorf("keychain store %q: %w", key, err)
		}
	}

	// SAFE: only store refresh_token if non-empty.
	if tok.RefreshToken != "" {
		if err := keychain.Set(keychain.KeyRefreshToken, tok.RefreshToken); err != nil {
			return fmt.Errorf("keychain store %q: %w", keychain.KeyRefreshToken, err)
		}
	} else {
		fmt.Println("WARNING: refresh_token absent from login response — not stored.")
		fmt.Println("         If no token exists in the keychain, re-run 'tt login'.")
	}

	fmt.Printf("✓ Credentials stored. token_type=%s expires_in=%ds\n",
		tok.TokenType, tok.ExpiresIn)
	fmt.Println("Run 'tt accounts' to verify.")
	return nil
}
