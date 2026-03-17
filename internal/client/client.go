package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theglove44/tastytrade-cli/config"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// familyLimiter holds a rate.Limiter per endpoint family.
type familyLimiter struct {
	orders       *rate.Limiter
	read         *rate.Limiter
	instruments  *rate.Limiter
	marketData   *rate.Limiter
	transactions *rate.Limiter
	auth         *rate.Limiter
	defaultLim   *rate.Limiter
}

func newFamilyLimiter(rl config.RateLimits) *familyLimiter {
	rps := func(r float64) *rate.Limiter {
		return rate.NewLimiter(rate.Limit(r), 1) // burst=1: no queue build-up
	}
	return &familyLimiter{
		orders:       rps(rl.Orders),
		read:         rps(rl.Read),
		instruments:  rps(rl.Instruments),
		marketData:   rps(rl.MarketData),
		transactions: rps(rl.Transactions),
		auth:         rps(rl.Auth),
		defaultLim:   rps(rl.Default),
	}
}

// Family constants used by callers to select the correct limiter.
const (
	FamilyOrders       = "orders"
	FamilyRead         = "read"
	FamilyInstruments  = "instruments"
	FamilyMarketData   = "market_data"
	FamilyTransactions = "transactions"
	FamilyAuth         = "auth"
)

func (fl *familyLimiter) get(family string) *rate.Limiter {
	switch family {
	case FamilyOrders:
		return fl.orders
	case FamilyRead:
		return fl.read
	case FamilyInstruments:
		return fl.instruments
	case FamilyMarketData:
		return fl.marketData
	case FamilyTransactions:
		return fl.transactions
	case FamilyAuth:
		return fl.auth
	default:
		return fl.defaultLim
	}
}

// Client is the TastyTrade HTTP client.
// It injects all required headers, applies per-family rate limiting,
// handles Retry-After on 429, and proactively refreshes the OAuth token.
type Client struct {
	cfg           *config.Config
	httpClient    *http.Client
	token         *tokenState
	limiter       *familyLimiter
	breaker       *CircuitBreaker
	log           *zap.Logger
	reqID         uint64
	reqIDMu       sync.Mutex
	authenticated bool // false for bootstrap clients (login flow)
}

// New creates a fully-authenticated Client.
// Credentials are loaded from the OS keychain on first use.
// The circuit breaker is initialised from env vars (or sensible defaults).
func New(cfg *config.Config, log *zap.Logger) *Client {
	maxOrders := envInt("TASTYTRADE_MAX_ORDERS_PER_HOUR", 10)
	return &Client{
		cfg:           cfg,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		token:         &tokenState{},
		limiter:       newFamilyLimiter(cfg.RateLimits),
		breaker:       NewCircuitBreaker(maxOrders, time.Hour),
		log:           log,
		authenticated: true,
	}
}

// NewUnauthenticated creates a Client that omits the Authorization header.
// Use this only for the login bootstrap flow (token exchange), where no token
// exists yet. All other middleware — rate limiting, structured logging, retry,
// Retry-After, metrics, X-Request-ID — behaves identically to New().
//
// Do NOT use this for any endpoint other than /oauth/token.
func NewUnauthenticated(cfg *config.Config, log *zap.Logger) *Client {
	return &Client{
		cfg:           cfg,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		token:         &tokenState{},
		limiter:       newFamilyLimiter(cfg.RateLimits),
		breaker:       NewCircuitBreaker(1, time.Hour), // irrelevant for auth-only client
		log:           log,
		authenticated: false,
	}
}

// AccessToken returns the current raw access token, calling EnsureToken first.
// For use by the account streamer ONLY — the wire protocol requires the bare
// token string, not the "Bearer <token>" form returned by authHeader().
// REST callers must use Do() instead.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if err := c.EnsureToken(ctx); err != nil {
		return "", fmt.Errorf("AccessToken: %w", err)
	}
	c.token.mu.RLock()
	defer c.token.mu.RUnlock()
	return c.token.accessToken, nil
}

// CheckOrderSafety runs all pre-order safety checks in spec-mandated order:
//  1. Kill switch (env + file)
//  2. Circuit breaker
//
// NLQ guard is a stub here — it is wired in when the streamer/balance
// polling layer is available (Phase 2).
// Returns a non-nil error if any check fails; callers must not proceed.
func (c *Client) CheckOrderSafety() error {
	// 1. Kill switch
	if halted, reason := KillSwitch(); halted {
		c.log.Warn("order blocked by kill switch", zap.String("reason", reason))
		return fmt.Errorf("kill switch active: %s", reason)
	}
	// 2. Circuit breaker
	if ok, reason := c.breaker.Allow(); !ok {
		c.log.Warn("order blocked by circuit breaker", zap.String("reason", reason))
		return fmt.Errorf("circuit breaker: %s", reason)
	}
	// 3. NLQ guard — stub until Phase 2 balance polling is wired
	// if err := c.nlqGuard.CheckOpening(ctx); err != nil { return err }

	// Metrics note:
	// OrdersSubmitted.WithLabelValues(strategy).Inc() — called by the live-submit
	// command after CheckOrderSafety passes, not here, so strategy label is available.
	// OrdersFilled, NLQDollars, OpenPositions — updated by streamer/balance poller (Phase 2).
	return nil
}

// BreakerState returns the human-readable circuit breaker state string.
// Used by startup logging and status commands.
func (c *Client) BreakerState() string {
	return c.breaker.State()
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func (c *Client) nextReqID() string {
	c.reqIDMu.Lock()
	c.reqID++
	id := c.reqID
	c.reqIDMu.Unlock()
	return fmt.Sprintf("req-%06d", id)
}

// RequestOptions controls per-request behaviour overrides.
// Pass the zero value (RequestOptions{}) for standard behaviour.
type RequestOptions struct {
	// SkipVersion suppresses the Accept-Version header for this request.
	// Required for unversioned endpoints: /api-quote-tokens, /market-sessions,
	// and all auth endpoints (/oauth/token).
	// Auth endpoints never use client.Do so this is primarily for quote tokens
	// and market sessions callers.
	SkipVersion bool

	// IdempotencyKey overrides the auto-generated UUID for the X-Idempotency-Key
	// header. Use this when the caller needs to record the key (e.g. in the order
	// intent log) before calling Do. If empty, a fresh UUID is generated.
	// Only applied to orders-family POST/PUT requests.
	IdempotencyKey string
}

// Do executes an HTTP request with full middleware:
//   - proactive token refresh
//   - per-family rate limiter (wait, not drop)
//   - User-Agent + Authorization + Accept headers
//   - Accept-Version (suppressed if opts.SkipVersion or not configured)
//   - X-Request-ID for log correlation
//   - structured request/response logging
//   - 401 single refresh-and-retry (no infinite loop — executed at most once)
//   - Retry-After on 429 with exponential backoff
//   - exponential backoff on 5xx (500/502/503/504)
//   - no retry on 4xx other than 401/429
//   - metrics: errors, latency, rate-limit hits
func (c *Client) Do(ctx context.Context, req *http.Request, family string, opts ...RequestOptions) (*http.Response, error) {
	var o RequestOptions
	if len(opts) > 0 {
		o = opts[0]
	}

	// 1. Proactive token refresh — only for authenticated clients
	if c.authenticated && family != FamilyAuth {
		if err := c.EnsureToken(ctx); err != nil {
			return nil, fmt.Errorf("client.Do: ensure token: %w", err)
		}
	}

	// 2. Per-family rate limit — wait in place, never drop
	lim := c.limiter.get(family)
	if err := lim.Wait(ctx); err != nil {
		return nil, fmt.Errorf("client.Do: rate limiter: %w", err)
	}

	// 3. Inject headers (applied before every attempt, refreshed after 401)
	reqID := c.nextReqID()
	// Generate an idempotency key once per Do call — reused across all retry
	// attempts so the server can deduplicate even if the network drops mid-flight.
	// Applied only to orders-family POST/PUT requests (submission endpoints).
	// Callers may supply a pre-generated key via opts.IdempotencyKey so the same
	// key can be recorded in the intent log before the request is dispatched.
	idempotencyKey := o.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	injectHeaders := func() {
		if c.authenticated {
			req.Header.Set("Authorization", c.token.authHeader())
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		req.Header.Set("Accept", "application/json")
		if !o.SkipVersion && c.cfg.APIVersion != "" {
			req.Header.Set("Accept-Version", c.cfg.APIVersion)
		}
		req.Header.Set("X-Request-ID", reqID)
		// Idempotency key: orders-family POST/PUT only (submit, replace, complex-order).
		if family == FamilyOrders && (req.Method == http.MethodPost || req.Method == http.MethodPut) {
			req.Header.Set("X-Idempotency-Key", idempotencyKey)
		}
	}
	injectHeaders()

	// 4. Execute with retry loop (429 and 5xx only)
	const maxAttempts = 3
	base := 500 * time.Millisecond
	tokenRefreshed := false // guard: 401 refresh-and-retry executes at most once

	for attempt := 0; attempt < maxAttempts; attempt++ {
		start := time.Now()
		c.log.Debug("request",
			zap.String("req_id", reqID),
			zap.String("method", req.Method),
			zap.String("url", req.URL.String()),
			zap.String("family", family),
			zap.Int("attempt", attempt+1),
			zap.String("idempotency_key", func() string {
				if family == FamilyOrders && (req.Method == http.MethodPost || req.Method == http.MethodPut) {
					return idempotencyKey
				}
				return ""
			}()),
		)

		resp, err := c.httpClient.Do(req)
		elapsed := time.Since(start)
		Metrics.RequestDuration.WithLabelValues(family, req.Method).Observe(elapsed.Seconds())

		if err != nil {
			c.log.Error("request error",
				zap.String("req_id", reqID),
				zap.Error(err),
				zap.Int("attempt", attempt+1),
			)
			if attempt+1 < maxAttempts {
				time.Sleep(base * time.Duration(1<<attempt))
				continue
			}
			return nil, fmt.Errorf("client.Do [%s]: %w", reqID, err)
		}

		c.log.Info("response",
			zap.String("req_id", reqID),
			zap.Int("status", resp.StatusCode),
			zap.String("family", family),
			zap.Duration("elapsed", elapsed),
		)

		switch resp.StatusCode {

		case http.StatusUnauthorized: // 401 — token may have expired mid-flight
			Metrics.APIErrors.WithLabelValues("401").Inc()
			resp.Body.Close()
			if !c.authenticated {
				// Unauthenticated bootstrap client — 401 means bad credentials, not stale token.
				return nil, fmt.Errorf("client.Do [%s]: 401 — check client_id, client_secret, and refresh_token", reqID)
			}
			if tokenRefreshed {
				// Already refreshed once this call — do not loop. Return error.
				return nil, fmt.Errorf("client.Do [%s]: 401 after token refresh — credentials may be invalid", reqID)
			}
			c.log.Warn("401 received — forcing token refresh and retrying once",
				zap.String("req_id", reqID),
			)
			// Force a full refresh regardless of the proactive window.
			c.token.mu.Lock()
			refreshErr := c.doTokenRefresh(ctx)
			c.token.mu.Unlock()
			if refreshErr != nil {
				return nil, fmt.Errorf("client.Do [%s]: token refresh after 401: %w", reqID, refreshErr)
			}
			tokenRefreshed = true
			// Rebuild Authorization header with the new token, then retry.
			injectHeaders()
			continue

		case http.StatusTooManyRequests: // 429
			Metrics.RateLimitHits.WithLabelValues(family).Inc()
			wait := parseRetryAfter(resp.Header.Get("Retry-After"), base, attempt)
			c.log.Warn("rate limited",
				zap.String("req_id", reqID),
				zap.String("family", family),
				zap.Duration("wait", wait),
				zap.Int("attempt", attempt+1),
			)
			resp.Body.Close()
			if attempt+1 < maxAttempts {
				time.Sleep(wait)
				continue
			}
			return nil, fmt.Errorf("client.Do [%s]: rate limited after %d attempts", reqID, maxAttempts)

		case 500, 502, 503, 504:
			Metrics.APIErrors.WithLabelValues(strconv.Itoa(resp.StatusCode)).Inc()
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.log.Error("server error",
				zap.String("req_id", reqID),
				zap.Int("status", resp.StatusCode),
				zap.ByteString("body", body),
				zap.Int("attempt", attempt+1),
			)
			if attempt+1 < maxAttempts {
				time.Sleep(base * time.Duration(1<<attempt))
				continue
			}
			return nil, fmt.Errorf("client.Do [%s]: server error %d after %d attempts: %s",
				reqID, resp.StatusCode, maxAttempts, body)

		default:
			// 4xx other than 401/429 (including 422 Unprocessable) — never retry.
			if resp.StatusCode >= 400 {
				Metrics.APIErrors.WithLabelValues(strconv.Itoa(resp.StatusCode)).Inc()
			}
			return resp, nil
		}
	}
	return nil, fmt.Errorf("client.Do [%s]: exhausted %d attempts", reqID, maxAttempts)
}

// parseRetryAfter parses the Retry-After header (integer seconds or HTTP-date).
// Falls back to exponential backoff if header is absent or unparseable.
func parseRetryAfter(header string, base time.Duration, attempt int) time.Duration {
	if header == "" {
		d := base * time.Duration(1<<attempt)
		if d > 60*time.Second {
			d = 60 * time.Second
		}
		return d
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if w := time.Until(t); w > 0 {
			return w
		}
		return 0
	}
	d := base * time.Duration(1<<attempt)
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}
