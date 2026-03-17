// Package tastytrade provides the Exchange implementation backed by the
// TastyTrade HTTP API.
//
// It wraps internal/client for all transport concerns (auth, rate limiting,
// retry, metrics, idempotency keys) and exposes clean domain-level methods
// that return typed models.
package tastytrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

// Exchange implements exchange.Exchange against the TastyTrade REST API.
type Exchange struct {
	cl      *client.Client
	baseURL string
}

// New creates a TastyTrade Exchange.
// cl must be a fully-authenticated client (created via client.New).
// baseURL is read from cfg.BaseURL in the caller (e.g. cmd/root.go).
func New(cl *client.Client, baseURL string) *Exchange {
	return &Exchange{cl: cl, baseURL: baseURL}
}

// Accounts returns all accounts for the authenticated user.
func (e *Exchange) Accounts(ctx context.Context) ([]models.Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.baseURL+"/customers/me/accounts", nil)
	if err != nil {
		return nil, fmt.Errorf("exchange.Accounts: build request: %w", err)
	}

	resp, err := e.cl.Do(ctx, req, client.FamilyRead)
	if err != nil {
		return nil, fmt.Errorf("exchange.Accounts: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange.Accounts: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.ItemsEnvelope[models.AccountListItem]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("exchange.Accounts: parse: %w", err)
	}

	out := make([]models.Account, 0, len(env.Data.Items))
	for _, item := range env.Data.Items {
		out = append(out, item.Account)
	}
	return out, nil
}

// Positions returns all open positions for the given account, fetching all
// pages automatically.
func (e *Exchange) Positions(ctx context.Context, accountID string) ([]models.Position, error) {
	var all []models.Position
	page := 0
	totalPages := 1

	for page < totalPages {
		url := fmt.Sprintf("%s/accounts/%s/positions", e.baseURL, accountID)
		if page > 0 {
			url = fmt.Sprintf("%s?page-offset=%d", url, page)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("exchange.Positions: build request page %d: %w", page, err)
		}

		resp, err := e.cl.Do(ctx, req, client.FamilyRead)
		if err != nil {
			return nil, fmt.Errorf("exchange.Positions: page %d: %w", page, err)
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("exchange.Positions: HTTP %d: %s", resp.StatusCode, data)
		}

		var env models.ItemsEnvelope[models.Position]
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("exchange.Positions: parse page %d: %w", page, err)
		}

		all = append(all, env.Data.Items...)
		if p := env.Data.Pagination; p != nil && p.TotalPages > 0 {
			totalPages = p.TotalPages
		}
		page++
	}
	return all, nil
}

// Orders returns all live orders for the given account, fetching all pages.
func (e *Exchange) Orders(ctx context.Context, accountID string) ([]models.Order, error) {
	var all []models.Order
	page := 0
	totalPages := 1

	for page < totalPages {
		url := fmt.Sprintf("%s/accounts/%s/orders/live", e.baseURL, accountID)
		if page > 0 {
			url = fmt.Sprintf("%s?page-offset=%d", url, page)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("exchange.Orders: build request page %d: %w", page, err)
		}

		resp, err := e.cl.Do(ctx, req, client.FamilyRead)
		if err != nil {
			return nil, fmt.Errorf("exchange.Orders: page %d: %w", page, err)
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("exchange.Orders: HTTP %d: %s", resp.StatusCode, data)
		}

		var env models.ItemsEnvelope[models.Order]
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("exchange.Orders: parse page %d: %w", page, err)
		}

		all = append(all, env.Data.Items...)
		if p := env.Data.Pagination; p != nil && p.TotalPages > 0 {
			totalPages = p.TotalPages
		}
		page++
	}
	return all, nil
}

// RecentOrders returns recent broker-facing order state for the account.
// It queries the search-orders endpoint and stops once limit results are collected.
func (e *Exchange) RecentOrders(ctx context.Context, accountID string, limit int) ([]models.Order, error) {
	if limit <= 0 {
		limit = 10
	}
	var all []models.Order
	page := 0
	totalPages := 1

	for page < totalPages && len(all) < limit {
		url := fmt.Sprintf("%s/accounts/%s/orders?sort=Desc&per-page=%d&page-offset=%d", e.baseURL, accountID, limit, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("exchange.RecentOrders: build request page %d: %w", page, err)
		}

		resp, err := e.cl.Do(ctx, req, client.FamilyRead)
		if err != nil {
			return nil, fmt.Errorf("exchange.RecentOrders: page %d: %w", page, err)
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("exchange.RecentOrders: HTTP %d: %s", resp.StatusCode, data)
		}

		var env models.ItemsEnvelope[models.Order]
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("exchange.RecentOrders: parse page %d: %w", page, err)
		}

		all = append(all, env.Data.Items...)
		if p := env.Data.Pagination; p != nil && p.TotalPages > 0 {
			totalPages = p.TotalPages
		}
		page++
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// Order returns one broker order by canonical broker order ID.
func (e *Exchange) Order(ctx context.Context, accountID, orderID string) (models.Order, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/accounts/%s/orders/%s", e.baseURL, accountID, orderID), nil)
	if err != nil {
		return models.Order{}, fmt.Errorf("exchange.Order: build request: %w", err)
	}

	resp, err := e.cl.Do(ctx, req, client.FamilyRead)
	if err != nil {
		return models.Order{}, fmt.Errorf("exchange.Order: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return models.Order{}, fmt.Errorf("exchange.Order: broker order %s not found in account %s; confirm the canonical broker order id and selected account", orderID, accountID)
	}
	if resp.StatusCode != http.StatusOK {
		return models.Order{}, fmt.Errorf("exchange.Order: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.Order]
	if err := json.Unmarshal(data, &env); err != nil {
		return models.Order{}, fmt.Errorf("exchange.Order: parse: %w", err)
	}
	if env.Data.ID == "" {
		return models.Order{}, fmt.Errorf("exchange.Order: empty broker order id in response for account %s", accountID)
	}
	if env.Data.ID != orderID {
		return models.Order{}, fmt.Errorf("exchange.Order: broker order id mismatch: requested %s got %s", orderID, env.Data.ID)
	}
	if env.Data.AccountNumber != "" && env.Data.AccountNumber != accountID {
		return models.Order{}, fmt.Errorf("exchange.Order: broker order %s belongs to account %s, not requested account %s", orderID, env.Data.AccountNumber, accountID)
	}
	return env.Data, nil
}

// DryRun submits the order to /orders/dry-run without placing a live trade.
// idempotencyKey is the pre-generated UUID recorded in the intent log by the
// command layer — passing it here ensures the logged key and the
// X-Idempotency-Key HTTP header are always identical.
// The safety gate (kill switch + circuit breaker) must be called by the command
// layer before invoking this method — it is not repeated here to avoid
// double-counting circuit breaker attempts.
func (e *Exchange) DryRun(ctx context.Context, accountID string, order models.NewOrder, idempotencyKey string) (models.DryRunResult, error) {
	body, err := json.Marshal(order)
	if err != nil {
		return models.DryRunResult{}, fmt.Errorf("exchange.DryRun: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/accounts/%s/orders/dry-run", e.baseURL, accountID),
		bytes.NewReader(body))
	if err != nil {
		return models.DryRunResult{}, fmt.Errorf("exchange.DryRun: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.cl.Do(ctx, req, client.FamilyOrders,
		client.RequestOptions{IdempotencyKey: idempotencyKey})
	if err != nil {
		return models.DryRunResult{}, fmt.Errorf("exchange.DryRun: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return models.DryRunResult{}, fmt.Errorf("exchange.DryRun: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.DryRunResult]
	if err := json.Unmarshal(data, &env); err != nil {
		return models.DryRunResult{}, fmt.Errorf("exchange.DryRun: parse: %w", err)
	}
	return env.Data, nil
}

// Submit routes a live order to /orders using the exact same payload shape as
// dry-run. The command layer is responsible for calling CheckOrderSafety,
// decision gating, and intent logging before invoking this method.
func (e *Exchange) Submit(ctx context.Context, accountID string, order models.NewOrder, idempotencyKey string) (models.SubmitResult, error) {
	body, err := json.Marshal(order)
	if err != nil {
		return models.SubmitResult{}, fmt.Errorf("exchange.Submit: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/accounts/%s/orders", e.baseURL, accountID),
		bytes.NewReader(body))
	if err != nil {
		return models.SubmitResult{}, fmt.Errorf("exchange.Submit: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.cl.Do(ctx, req, client.FamilyOrders,
		client.RequestOptions{IdempotencyKey: idempotencyKey})
	if err != nil {
		return models.SubmitResult{}, fmt.Errorf("exchange.Submit: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return models.SubmitResult{}, fmt.Errorf("exchange.Submit: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.SubmitResult]
	if err := json.Unmarshal(data, &env); err != nil {
		return models.SubmitResult{}, fmt.Errorf("exchange.Submit: parse: %w", err)
	}
	return env.Data, nil
}

// QuoteToken fetches a fresh DXLink authentication token.
// The /api-quote-tokens endpoint is unversioned — SkipVersion:true is mandatory.
// A new token must be retrieved before every DXLink connect and reconnect.
func (e *Exchange) QuoteToken(ctx context.Context) (models.QuoteToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.baseURL+"/api-quote-tokens", nil)
	if err != nil {
		return models.QuoteToken{}, fmt.Errorf("exchange.QuoteToken: build request: %w", err)
	}

	resp, err := e.cl.Do(ctx, req, client.FamilyMarketData,
		client.RequestOptions{SkipVersion: true})
	if err != nil {
		return models.QuoteToken{}, fmt.Errorf("exchange.QuoteToken: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return models.QuoteToken{}, fmt.Errorf("exchange.QuoteToken: HTTP %d: %s", resp.StatusCode, data)
	}

	var env models.DataEnvelope[models.QuoteToken]
	if err := json.Unmarshal(data, &env); err != nil {
		return models.QuoteToken{}, fmt.Errorf("exchange.QuoteToken: parse: %w", err)
	}
	return env.Data, nil
}
