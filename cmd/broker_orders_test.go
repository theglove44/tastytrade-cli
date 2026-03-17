package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

type brokerOrdersTestExchange struct {
	liveOrders     []models.Order
	recentOrders   []models.Order
	recentLimit    int
	orderErr       error
	orderAccountID string
}

func (b *brokerOrdersTestExchange) Accounts(context.Context) ([]models.Account, error) {
	return []models.Account{{AccountNumber: "TEST123"}}, nil
}
func (b *brokerOrdersTestExchange) Positions(context.Context, string) ([]models.Position, error) {
	return nil, nil
}
func (b *brokerOrdersTestExchange) Orders(context.Context, string) ([]models.Order, error) {
	return b.liveOrders, nil
}
func (b *brokerOrdersTestExchange) RecentOrders(_ context.Context, _ string, limit int) ([]models.Order, error) {
	b.recentLimit = limit
	return b.recentOrders, nil
}
func (b *brokerOrdersTestExchange) Order(_ context.Context, accountID, orderID string) (models.Order, error) {
	b.orderAccountID = accountID
	if b.orderErr != nil {
		return models.Order{}, b.orderErr
	}
	for _, order := range append(append([]models.Order{}, b.liveOrders...), b.recentOrders...) {
		if order.ID == orderID {
			return order, nil
		}
	}
	return models.Order{}, fmt.Errorf("broker order %s not found in account %s; confirm the canonical broker order id and selected account", orderID, accountID)
}
func (b *brokerOrdersTestExchange) DryRun(context.Context, string, models.NewOrder, string) (models.DryRunResult, error) {
	return models.DryRunResult{}, nil
}
func (b *brokerOrdersTestExchange) Submit(context.Context, string, models.NewOrder, string) (models.SubmitResult, error) {
	return models.SubmitResult{}, nil
}
func (b *brokerOrdersTestExchange) QuoteToken(context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

func sampleBrokerOrder(status string) models.Order {
	now := time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC)
	filled := now.Add(-time.Minute)
	return models.Order{
		ID:            "ord-1",
		AccountNumber: "TEST123",
		Status:        status,
		OrderType:     "Limit",
		TimeInForce:   "Day",
		Price:         decimal.RequireFromString("1.23"),
		PriceEffect:   "Debit",
		ReceivedAt:    now.Add(-2 * time.Minute),
		UpdatedAt:     now,
		FilledAt:      &filled,
		Legs: []models.OrderLeg{{
			InstrumentType: "Equity",
			Symbol:         "AAPL",
			Quantity:       decimal.RequireFromString("1"),
			Action:         "Buy to Open",
		}},
	}
}

func sampleBrokerOrderWithoutOptionalFields(status string) models.Order {
	return models.Order{
		ID:            "ord-2",
		AccountNumber: "TEST123",
		Status:        status,
		OrderType:     "Market",
		TimeInForce:   "Day",
		Legs: []models.OrderLeg{{
			Symbol:   "SPY",
			Quantity: decimal.RequireFromString("2"),
			Action:   "Sell to Close",
		}},
	}
}

func sampleBrokerOrderWithFillContext(status string) models.Order {
	order := sampleBrokerOrder(status)
	order.Legs = []models.OrderLeg{{
		InstrumentType: "Equity",
		Symbol:         "AAPL",
		Quantity:       decimal.RequireFromString("1"),
		Action:         "Buy to Open",
		FillQuantity:   decimal.RequireFromString("1"),
		FillPrice:      decimal.RequireFromString("1.21"),
	}}
	return order
}

func TestBuildBrokerOrderView_ShapesKeyFields(t *testing.T) {
	view := buildBrokerOrderView(sampleBrokerOrder("Filled"))
	if view.ID != "ord-1" || view.Status != "Filled" || view.Price != "1.23" {
		t.Fatalf("view = %+v, want key order fields shaped", view)
	}
	if len(view.Legs) != 1 || view.Legs[0].Symbol != "AAPL" {
		t.Fatalf("legs = %+v, want shaped leg summary", view.Legs)
	}
	if view.UpdatedAt == "" || view.FilledAt == "" {
		t.Fatalf("timestamps = %+v, want updated and filled timestamps", view)
	}
}

func TestRunBrokerOrdersRecent_JSON(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON, oldLimit := cfg, ex, flagJSON, flagBrokerOrdersLimit
	defer func() { cfg, ex, flagJSON, flagBrokerOrdersLimit = oldCfg, oldEx, oldFlagJSON, oldLimit }()

	stub := &brokerOrdersTestExchange{recentOrders: []models.Order{sampleBrokerOrder("Routed")}}
	cfg = &config.Config{AccountID: "TEST123"}
	ex = stub
	flagJSON = true
	flagBrokerOrdersLimit = 5

	stdout := captureStdout(t, func() {
		if err := runBrokerOrdersRecent(context.Background()); err != nil {
			t.Fatalf("runBrokerOrdersRecent: %v", err)
		}
	})
	if stub.recentLimit != 5 {
		t.Fatalf("recentLimit = %d, want 5", stub.recentLimit)
	}
	for _, want := range []string{"\"source\": \"recent(limit=5)\"", "\"status\": \"Routed\"", "\"updated_at\":"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestRunBrokerOrdersLive_HumanReadable(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON := cfg, ex, flagJSON
	defer func() { cfg, ex, flagJSON = oldCfg, oldEx, oldFlagJSON }()

	stub := &brokerOrdersTestExchange{liveOrders: []models.Order{sampleBrokerOrder("Live")}}
	cfg = &config.Config{AccountID: "TEST123"}
	ex = stub
	flagJSON = false

	stdout := captureStdout(t, func() {
		if err := runBrokerOrdersLive(context.Background()); err != nil {
			t.Fatalf("runBrokerOrdersLive: %v", err)
		}
	})
	for _, want := range []string{"BROKER ORDERS (live)", "Live", "AAPL"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestRenderBrokerOrderDetail_HumanReadable(t *testing.T) {
	stdout := captureStdout(t, func() {
		if err := renderBrokerOrderDetail("TEST123", sampleBrokerOrderWithFillContext("Filled")); err != nil {
			t.Fatalf("renderBrokerOrderDetail: %v", err)
		}
	})
	for _, want := range []string{
		"BROKER ORDER DETAIL",
		"  order:",
		"    account=TEST123",
		"    id=ord-1",
		"    status=Filled",
		"  pricing:",
		"    price=1.23",
		"  timestamps:",
		"    filled_at=2026-03-15T12:59:00Z",
		"  legs:",
		"    1) AAPL",
		"       action=Buy to Open quantity=1",
		"       instrument_type=Equity",
		"       fill_context:",
		"         fill_quantity=1",
		"         average_fill_price=1.21",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestRenderBrokerOrderDetail_OmitsAbsentOptionalFields(t *testing.T) {
	stdout := captureStdout(t, func() {
		if err := renderBrokerOrderDetail("TEST123", sampleBrokerOrderWithoutOptionalFields("Cancelled")); err != nil {
			t.Fatalf("renderBrokerOrderDetail: %v", err)
		}
	})
	for _, want := range []string{"status=Cancelled", "type=Market", "    1) SPY", "action=Sell to Close quantity=2"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
	for _, unwanted := range []string{"filled_at=", "cancelled_at=", "instrument_type=", "fill_context:", "average_fill_price=", "received_at= updated_at="} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("stdout = %q, unexpected %q", stdout, unwanted)
		}
	}
}

func TestRunBrokerOrdersDetail_JSON(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON, oldID := cfg, ex, flagJSON, flagBrokerOrderID
	defer func() { cfg, ex, flagJSON, flagBrokerOrderID = oldCfg, oldEx, oldFlagJSON, oldID }()

	stub := &brokerOrdersTestExchange{liveOrders: []models.Order{sampleBrokerOrder("Filled")}}
	cfg = &config.Config{AccountID: "TEST123"}
	ex = stub
	flagJSON = true
	flagBrokerOrderID = "ord-1"

	stdout := captureStdout(t, func() {
		if err := runBrokerOrdersDetail(context.Background()); err != nil {
			t.Fatalf("runBrokerOrdersDetail: %v", err)
		}
	})
	if stub.orderAccountID != "TEST123" {
		t.Fatalf("orderAccountID = %q, want TEST123", stub.orderAccountID)
	}
	for _, want := range []string{"\"account_number\": \"TEST123\"", "\"id\": \"ord-1\"", "\"status\": \"Filled\"", "\"order\":"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "\"source\":") || strings.Contains(stdout, "\"orders\":") {
		t.Fatalf("stdout = %q, contains list-only broker-orders fields", stdout)
	}
}

func TestRunBrokerOrdersDetail_NotFound(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON, oldID := cfg, ex, flagJSON, flagBrokerOrderID
	defer func() { cfg, ex, flagJSON, flagBrokerOrderID = oldCfg, oldEx, oldFlagJSON, oldID }()

	cfg = &config.Config{AccountID: "TEST123"}
	ex = &brokerOrdersTestExchange{}
	flagJSON = false
	flagBrokerOrderID = "missing-1"

	err := runBrokerOrdersDetail(context.Background())
	if err == nil {
		t.Fatal("runBrokerOrdersDetail error = nil, want not found error")
	}
	for _, want := range []string{"broker-orders detail:", "missing-1", "TEST123", "canonical broker order id", "selected account"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, missing %q", err.Error(), want)
		}
	}
}

func TestRunBrokerOrdersDetail_RejectsLikelyLocalSubmitIdentity(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON, oldID := cfg, ex, flagJSON, flagBrokerOrderID
	defer func() { cfg, ex, flagJSON, flagBrokerOrderID = oldCfg, oldEx, oldFlagJSON, oldID }()

	cfg = &config.Config{AccountID: "TEST123"}
	ex = &brokerOrdersTestExchange{}
	flagJSON = false
	flagBrokerOrderID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	err := runBrokerOrdersDetail(context.Background())
	if err == nil {
		t.Fatal("runBrokerOrdersDetail error = nil, want local-id rejection")
	}
	for _, want := range []string{"broker-orders detail:", "canonical broker order id required", "local submit identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, missing %q", err.Error(), want)
		}
	}
}

func TestRunBrokerOrdersDetail_AccountMismatchError(t *testing.T) {
	oldCfg, oldEx, oldFlagJSON, oldID := cfg, ex, flagJSON, flagBrokerOrderID
	defer func() { cfg, ex, flagJSON, flagBrokerOrderID = oldCfg, oldEx, oldFlagJSON, oldID }()

	cfg = &config.Config{AccountID: "TEST123"}
	ex = &brokerOrdersTestExchange{orderErr: fmt.Errorf("broker order ord-1 belongs to account OTHER123, not requested account TEST123")}
	flagJSON = false
	flagBrokerOrderID = "ord-1"

	err := runBrokerOrdersDetail(context.Background())
	if err == nil {
		t.Fatal("runBrokerOrdersDetail error = nil, want account mismatch error")
	}
	for _, want := range []string{"broker-orders detail:", "belongs to account OTHER123", "requested account TEST123"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, missing %q", err.Error(), want)
		}
	}
}
