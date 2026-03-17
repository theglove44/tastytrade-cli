package cmd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

type brokerOrdersTestExchange struct {
	liveOrders   []models.Order
	recentOrders []models.Order
	recentLimit  int
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
