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

	var env models.ItemsEnvelope[models.Account]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("exchange.Accounts: parse: %w", err)
	}
	return env.Data.Items, nil
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
