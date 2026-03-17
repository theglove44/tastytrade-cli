package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

func TestRunSubmitStateCompare_JSON_FilteredByOutcomeAndAccount(t *testing.T) {
	r := setupSubmitStateTest(t)
	oldCfg, oldEx, oldFlagJSON, oldLimit, oldAccount, oldOutcome := cfg, ex, flagJSON, flagSubmitStateCompareLimit, flagSubmitStateCompareAccount, flagSubmitStateCompareOutcome
	defer func() {
		cfg, ex, flagJSON, flagSubmitStateCompareLimit, flagSubmitStateCompareAccount, flagSubmitStateCompareOutcome = oldCfg, oldEx, oldFlagJSON, oldLimit, oldAccount, oldOutcome
	}()

	matchingOrder := comparableOrder(t, "BROKER-1", "Live", comparableNewOrder("SPY 250320C00580000", "1.2", "Credit"))
	otherOrder := comparableOrder(t, "BROKER-2", "Filled", comparableNewOrder("AAPL", "0", "Debit"))

	hash, err := brokerOrderHash(matchingOrder)
	if err != nil {
		t.Fatalf("brokerOrderHash: %v", err)
	}
	identity, err := deriveSubmitIdentity("ACCT-1", "intent-1", hash)
	if err != nil {
		t.Fatalf("deriveSubmitIdentity: %v", err)
	}
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}

	cfg = &config.Config{}
	ex = &submitStateCompareTestExchange{liveOrders: []models.Order{matchingOrder, otherOrder}}
	flagJSON = true
	flagSubmitStateCompareLimit = 4
	flagSubmitStateCompareAccount = "ACCT-1"
	flagSubmitStateCompareOutcome = ComparisonPlausibleMatch

	stdout := captureStdout(t, func() {
		if err := runSubmitStateCompare(context.Background()); err != nil {
			t.Fatalf("runSubmitStateCompare: %v", err)
		}
	})
	for _, want := range []string{"\"account_number\": \"ACCT-1\"", "\"outcome_filter\": \"local_present_broker_match\"", "\"count\": 1", "\"broker_order_id\": \"BROKER-1\"", "\"recommended_actions\":"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "BROKER-2") {
		t.Fatalf("stdout = %q, unexpected filtered-out broker order", stdout)
	}
}

func TestValidateSubmitStateCompareOutcomeFilter(t *testing.T) {
	if err := validateSubmitStateCompareOutcomeFilter(ComparisonAmbiguous); err != nil {
		t.Fatalf("valid filter rejected: %v", err)
	}
	if err := validateSubmitStateCompareOutcomeFilter("nope"); err == nil {
		t.Fatal("invalid outcome filter accepted")
	}
}

func comparableNewOrder(symbol, price, effect string) models.NewOrder {
	return models.NewOrder{
		OrderType:   "Limit",
		TimeInForce: "Day",
		Price:       price,
		PriceEffect: effect,
		Legs:        []models.NewOrderLeg{{InstrumentType: "Equity", Symbol: symbol, Quantity: 1, Action: "Buy to Open"}},
	}
}
