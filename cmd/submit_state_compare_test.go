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

type submitStateCompareTestExchange struct {
	liveOrders   []models.Order
	recentOrders []models.Order
	recentLimit  int
}

func (s *submitStateCompareTestExchange) Accounts(context.Context) ([]models.Account, error) {
	return []models.Account{{AccountNumber: "ACCT-1"}}, nil
}
func (s *submitStateCompareTestExchange) Positions(context.Context, string) ([]models.Position, error) {
	return nil, nil
}
func (s *submitStateCompareTestExchange) Orders(context.Context, string) ([]models.Order, error) {
	return s.liveOrders, nil
}
func (s *submitStateCompareTestExchange) RecentOrders(_ context.Context, _ string, limit int) ([]models.Order, error) {
	s.recentLimit = limit
	return s.recentOrders, nil
}
func (s *submitStateCompareTestExchange) DryRun(context.Context, string, models.NewOrder, string) (models.DryRunResult, error) {
	return models.DryRunResult{}, nil
}
func (s *submitStateCompareTestExchange) Submit(context.Context, string, models.NewOrder, string) (models.SubmitResult, error) {
	return models.SubmitResult{}, nil
}
func (s *submitStateCompareTestExchange) QuoteToken(context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

func comparableOrder(t *testing.T, id, status string, order models.NewOrder) models.Order {
	t.Helper()
	price := decimal.RequireFromString(order.Price)
	updated := time.Date(2026, 3, 15, 12, 30, 0, 0, time.UTC)
	mapped := models.Order{
		ID:            id,
		AccountNumber: "ACCT-1",
		Status:        status,
		OrderType:     order.OrderType,
		TimeInForce:   order.TimeInForce,
		Price:         price,
		PriceEffect:   order.PriceEffect,
		ReceivedAt:    updated.Add(-time.Minute),
		UpdatedAt:     updated,
	}
	for _, leg := range order.Legs {
		mapped.Legs = append(mapped.Legs, models.OrderLeg{
			InstrumentType: leg.InstrumentType,
			Symbol:         leg.Symbol,
			Quantity:       decimal.NewFromInt(int64(leg.Quantity)),
			Action:         leg.Action,
		})
	}
	return mapped
}

func TestCompareLocalSubmitStateToBroker_PlausibleMatch(t *testing.T) {
	order := models.NewOrder{
		OrderType:   "Limit",
		TimeInForce: "Day",
		Price:       "1.2",
		PriceEffect: "Credit",
		Legs:        []models.NewOrderLeg{{InstrumentType: "Equity Option", Symbol: "SPY 250320C00580000", Quantity: 1, Action: "Sell to Open"}},
	}
	hash, err := canonicalOrderHash(order)
	if err != nil {
		t.Fatalf("canonicalOrderHash: %v", err)
	}
	results, broker := compareLocalSubmitStateToBroker("ACCT-1", []SubmitStateRecordView{{SubmitIdentity: "sid-1", AccountID: "ACCT-1", OrderHash: hash, State: string(SubmitIdentityInFlight)}}, []models.Order{comparableOrder(t, "BROKER-1", "Live", order)}, nil)
	if len(broker) != 1 || len(results) != 1 {
		t.Fatalf("broker=%d results=%d, want 1/1", len(broker), len(results))
	}
	if results[0].Outcome != ComparisonPlausibleMatch || results[0].BrokerOrderID != "BROKER-1" {
		t.Fatalf("result = %+v, want plausible match to BROKER-1", results[0])
	}
}

func TestCompareLocalSubmitStateToBroker_InFlightNoBrokerMatch(t *testing.T) {
	results, _ := compareLocalSubmitStateToBroker("ACCT-1", []SubmitStateRecordView{{SubmitIdentity: "sid-1", AccountID: "ACCT-1", OrderHash: "hash-1", State: string(SubmitIdentityInFlight)}}, nil, nil)
	if len(results) != 1 || results[0].Outcome != ComparisonLocalNoBrokerMatch {
		t.Fatalf("results = %+v, want in-flight/no-broker outcome", results)
	}
}

func TestCompareLocalSubmitStateToBroker_BrokerOnly(t *testing.T) {
	order := models.NewOrder{
		OrderType:   "Market",
		TimeInForce: "Day",
		Price:       "0",
		PriceEffect: "Debit",
		Legs:        []models.NewOrderLeg{{InstrumentType: "Equity", Symbol: "AAPL", Quantity: 1, Action: "Buy to Open"}},
	}
	results, _ := compareLocalSubmitStateToBroker("ACCT-1", nil, []models.Order{comparableOrder(t, "BROKER-2", "Filled", order)}, nil)
	if len(results) != 1 || results[0].Outcome != ComparisonBrokerNoLocalState {
		t.Fatalf("results = %+v, want broker-only outcome", results)
	}
}

func TestCompareLocalSubmitStateToBroker_AmbiguousMultipleBrokerMatches(t *testing.T) {
	order := models.NewOrder{
		OrderType:   "Limit",
		TimeInForce: "Day",
		Price:       "2.5",
		PriceEffect: "Debit",
		Legs:        []models.NewOrderLeg{{InstrumentType: "Equity", Symbol: "QQQ", Quantity: 1, Action: "Buy to Open"}},
	}
	hash, err := canonicalOrderHash(order)
	if err != nil {
		t.Fatalf("canonicalOrderHash: %v", err)
	}
	results, _ := compareLocalSubmitStateToBroker(
		"ACCT-1",
		[]SubmitStateRecordView{{SubmitIdentity: "sid-1", AccountID: "ACCT-1", OrderHash: hash, State: string(SubmitIdentitySubmitted)}},
		[]models.Order{comparableOrder(t, "BROKER-1", "Live", order), comparableOrder(t, "BROKER-2", "Filled", order)},
		nil,
	)
	if len(results) == 0 || results[0].Outcome != ComparisonAmbiguous {
		t.Fatalf("results = %+v, want ambiguous outcome", results)
	}
}

func TestRunSubmitStateCompare_JSON(t *testing.T) {
	r := setupSubmitStateTest(t)
	oldCfg, oldEx, oldFlagJSON, oldLimit := cfg, ex, flagJSON, flagSubmitStateCompareLimit
	defer func() { cfg, ex, flagJSON, flagSubmitStateCompareLimit = oldCfg, oldEx, oldFlagJSON, oldLimit }()

	order := models.NewOrder{
		OrderType:   "Limit",
		TimeInForce: "Day",
		Price:       "1.2",
		PriceEffect: "Credit",
		Legs:        []models.NewOrderLeg{{InstrumentType: "Equity Option", Symbol: "SPY 250320C00580000", Quantity: 1, Action: "Sell to Open"}},
	}
	hash, err := canonicalOrderHash(order)
	if err != nil {
		t.Fatalf("canonicalOrderHash: %v", err)
	}
	identity, err := deriveSubmitIdentity("ACCT-1", "intent-1", hash)
	if err != nil {
		t.Fatalf("deriveSubmitIdentity: %v", err)
	}
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}

	stub := &submitStateCompareTestExchange{liveOrders: []models.Order{comparableOrder(t, "BROKER-1", "Live", order)}}
	cfg = &config.Config{AccountID: "ACCT-1"}
	ex = stub
	flagJSON = true
	flagSubmitStateCompareLimit = 7

	stdout := captureStdout(t, func() {
		if err := runSubmitStateCompare(context.Background()); err != nil {
			t.Fatalf("runSubmitStateCompare: %v", err)
		}
	})
	if stub.recentLimit != 7 {
		t.Fatalf("recentLimit = %d, want 7", stub.recentLimit)
	}
	for _, want := range []string{"\"advisory\": \"advisory_manual_only\"", "\"outcome\": \"local_present_broker_match\"", "\"broker_order_id\": \"BROKER-1\""} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}
