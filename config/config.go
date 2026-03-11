// Package config loads runtime configuration from environment variables.
// All credentials are stored in the OS keychain (see internal/keychain).
// This file must never hold client_secret or refresh_token values.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// TokenTTL is the access_token lifetime returned by /oauth/token (expires_in=900).
	TokenTTL = 15 * time.Minute

	// RefreshThreshold is how early we proactively refresh before expiry.
	// Set to 12 min so there is a 3-min buffer against clock skew or slow networks.
	RefreshThreshold = 12 * time.Minute

	ProdBaseURL    = "https://api.tastytrade.com"
	SandboxBaseURL = "https://api.cert.tastyworks.com" // NOTE: tastyworks.com domain, not tastytrade.com

	AccountStreamerURL = "wss://streamer.tastytrade.com"
	DXLinkBaseURL      = "wss://tasty-openapi-ws.dxfeed.com/realtime" // may be overridden by /api-quote-tokens response
)

// RateLimits holds per-family request-per-second ceilings.
// Default is the fallback for families not explicitly configured.
// Orders is a hard cap — never raise above 1.0.
type RateLimits struct {
	Default      float64 // fallback for families without an explicit override
	Orders       float64 // submit + dry-run — hard cap at 1.0, never raise
	Read         float64 // balances, positions, accounts
	Instruments  float64 // option chains, equities, symbol search
	MarketData   float64 // REST quotes, market metrics, market sessions
	Transactions float64 // transaction history
	Auth         float64 // /oauth/token only — proactive refresh = <<1 per 15 min
}

// DefaultRateLimits returns conservative production-safe defaults.
// These are community-observed values — tastytrade does not publish rate limits.
func DefaultRateLimits() RateLimits {
	return RateLimits{
		Default:      2.0,
		Orders:       1.0, // hard cap — do not raise
		Read:         2.0,
		Instruments:  2.0,
		MarketData:   2.0,
		Transactions: 1.0,
		Auth:         0.1, // ~1 per 10s absolute max; normal cadence is 1 per 12 min
	}
}

// Config is the full runtime configuration for the CLI.
type Config struct {
	BaseURL            string
	AccountID          string
	ClientID           string
	UserAgent          string
	APIVersion         string // Accept-Version header value; empty = omit (use latest)
	AccountStreamerURL string // WebSocket endpoint for the account streamer
	DXLinkURL          string // DXLink market data WS endpoint (overridden by QuoteToken response)

	RateLimits RateLimits

	// LiveTrading is true ONLY when both the env var is "true" AND the
	// BaseURL points to the production host. Both conditions must hold.
	// This gate is evaluated at load time and is immutable for the process lifetime.
	LiveTrading bool

	// KillSwitch is evaluated at every order submission path — not just at startup.
	// See internal/client.KillSwitch() for the runtime check.
}

// Load reads Config from environment variables.
// Credentials (client_secret, refresh_token) are not read here — see internal/keychain.
func Load() (*Config, error) {
	baseURL := os.Getenv("TASTYTRADE_BASE_URL")
	if baseURL == "" {
		baseURL = SandboxBaseURL
	}

	clientID := os.Getenv("TASTYTRADE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("TASTYTRADE_CLIENT_ID is required")
	}

	userAgent := envOr("TASTYTRADE_USER_AGENT", "tastytrade-cli/1.0.0")
	apiVersion := os.Getenv("TASTYTRADE_API_VERSION") // empty = omit header

	rl := DefaultRateLimits()
	if v := envFloat("TASTYTRADE_RATE_DEFAULT_RPS"); v > 0 {
		rl.Default = v
	}
	if v := envFloat("TASTYTRADE_RATE_ORDERS_RPS"); v > 0 {
		// Hard cap: orders family can never exceed 1.0 regardless of config.
		if v > 1.0 {
			v = 1.0
		}
		rl.Orders = v
	}
	if v := envFloat("TASTYTRADE_RATE_READ_RPS"); v > 0 {
		rl.Read = v
	}
	if v := envFloat("TASTYTRADE_RATE_INSTRUMENTS_RPS"); v > 0 {
		rl.Instruments = v
	}
	if v := envFloat("TASTYTRADE_RATE_MARKETDATA_RPS"); v > 0 {
		rl.MarketData = v
	}
	if v := envFloat("TASTYTRADE_RATE_TRANSACTIONS_RPS"); v > 0 {
		rl.Transactions = v
	}

	liveEnv := strings.ToLower(os.Getenv("TASTYTRADE_LIVE_TRADING")) == "true"
	isProd := strings.Contains(baseURL, "api.tastytrade.com") &&
		!strings.Contains(baseURL, "cert")
	liveTrading := liveEnv && isProd

	return &Config{
		BaseURL:            baseURL,
		AccountID:          os.Getenv("TASTYTRADE_ACCOUNT_ID"),
		ClientID:           clientID,
		UserAgent:          userAgent,
		APIVersion:         apiVersion,
		AccountStreamerURL: envOr("TASTYTRADE_ACCOUNT_STREAMER_URL", AccountStreamerURL),
		DXLinkURL:          envOr("TASTYTRADE_DXLINK_URL", DXLinkBaseURL),
		RateLimits:         rl,
		LiveTrading:        liveTrading,
	}, nil
}

// IsProd returns true when BaseURL points at the production host.
func (c *Config) IsProd() bool {
	return strings.Contains(c.BaseURL, "api.tastytrade.com") &&
		!strings.Contains(c.BaseURL, "cert")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envFloat(key string) float64 {
	v := os.Getenv(key)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}
