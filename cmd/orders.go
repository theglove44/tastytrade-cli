package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	iclient "github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/intentlog"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

var ordersCmd = &cobra.Command{
	Use:   "orders",
	Short: "List live (open) orders",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrders(cmd.Context())
	},
}

// OrdersOutput is the stable --json schema.
type OrdersOutput struct {
	AccountNumber string         `json:"account_number"`
	Orders        []OrderSummary `json:"orders"`
	Count         int            `json:"count"`
}

type OrderSummary struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	OrderType   string          `json:"order_type"`
	TimeInForce string          `json:"time_in_force"`
	Price       decimal.Decimal `json:"price"`
	PriceEffect string          `json:"price_effect"`
	Legs        []LegSummary    `json:"legs"`
	ReceivedAt  string          `json:"received_at"`
}

type LegSummary struct {
	Symbol         string `json:"symbol"`
	InstrumentType string `json:"instrument_type"`
	Action         string `json:"action"`
	Quantity       string `json:"quantity"`
}

func runOrders(ctx context.Context) error {
	accountID := cfg.AccountID
	if accountID == "" {
		return fmt.Errorf("orders: TASTYTRADE_ACCOUNT_ID is not set")
	}

	items, err := ex.Orders(ctx, accountID)
	if err != nil {
		return fmt.Errorf("orders: %w", err)
	}

	if flagJSON {
		out := OrdersOutput{AccountNumber: accountID, Count: len(items)}
		for _, o := range items {
			out.Orders = append(out.Orders, buildOrderSummary(o))
		}
		return printJSON(out)
	}

	if len(items) == 0 {
		fmt.Println("No live orders.")
		return nil
	}
	fmt.Printf("%-12s %-10s %-10s %-8s %s\n", "ID", "STATUS", "TYPE", "PRICE", "LEGS")
	for _, o := range items {
		legs := make([]string, len(o.Legs))
		for i, l := range o.Legs {
			legs[i] = fmt.Sprintf("%s %s x%s", l.Action, l.Symbol, l.Quantity.String())
		}
		fmt.Printf("%-12s %-10s %-10s %-8s %s\n",
			o.ID, o.Status, o.OrderType, o.Price.String(),
			strings.Join(legs, " | "))
	}
	return nil
}

// ── Submit command ───────────────────────────────────────────────────────────

var (
	flagSubmitFile  string    // path to JSON file with NewOrder payload
	flagSubmitYes   bool      // explicit non-interactive acknowledgement for live submit
	flagDryRunFile  string    // path to JSON file with NewOrder payload
	submitConfirmIn io.Reader = os.Stdin
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a live order",
	Long: `Submits an order to /orders and may route it to a live venue.

Use only when live trading is explicitly enabled.
The order payload must be passed as a JSON file:
  tt submit --file order.json [--json] [--yes]

The JSON shape is identical to tt dry-run. Consider validating with dry-run first.

Live submit is fail-closed behind the full safety chain:
  live mode -> order safety -> reconcile decision gate -> operator confirmation -> final pre-submit policy -> duplicate-submit protection.

In --json mode, --yes is required to acknowledge live submission non-interactively.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmit(cmd.Context())
	},
}

func init() {
	submitCmd.Flags().StringVar(&flagSubmitFile, "file", "",
		"Path to JSON file containing the NewOrder payload (required)")
	submitCmd.Flags().BoolVar(&flagSubmitYes, "yes", false,
		"Required with --json to acknowledge live order submission non-interactively")
	_ = submitCmd.MarkFlagRequired("file")

	dryRunCmd.Flags().StringVar(&flagDryRunFile, "file", "",
		"Path to JSON file containing the NewOrder payload (required)")
	_ = dryRunCmd.MarkFlagRequired("file")
}

// SubmitOutput is the stable --json schema for the first live-submit path.
type SubmitOutput struct {
	Submitted          bool             `json:"submitted"`
	Warnings           []DryRunErrorOut `json:"warnings"`
	BuyingPowerEffect  BPEffectOut      `json:"buying_power_effect"`
	Order              OrderSummary     `json:"order"`
	DecisionGateStatus string           `json:"decision_gate_status,omitempty"`
}

func runSubmit(ctx context.Context) error {
	accountID := cfg.AccountID
	if accountID == "" {
		return fmt.Errorf("submit: TASTYTRADE_ACCOUNT_ID is not set")
	}
	if !cfg.LiveTrading {
		return fmt.Errorf("submit blocked: live trading is not enabled (set TASTYTRADE_LIVE_TRADING=true against production intentionally)")
	}

	safetyErr := cl.CheckOrderSafety()
	if safetyErr != nil {
		return fmt.Errorf("submit blocked: %w", safetyErr)
	}

	gateView := currentDecisionGate("submit", rec)
	if !flagJSON {
		emitDecisionGateHumanMessage(gateView)
	}
	gateErr := enforceDecisionGate("submit", rec, logger)
	if gateErr != nil {
		return gateErr
	}

	order, err := parseOrderFile(flagSubmitFile, "submit")
	if err != nil {
		return err
	}
	idemKey := uuid.NewString()
	confirmation, err := confirmLiveSubmit(accountID, order, idemKey)
	if err != nil {
		return err
	}

	policyResult := EvaluatePreSubmitPolicy(PreSubmitPolicyInput{
		Config:            cfg,
		AccountID:         accountID,
		IntentID:          idemKey,
		Order:             order,
		OrderHash:         confirmation.OrderHash,
		SafetyErr:         safetyErr,
		DecisionView:      gateView,
		DecisionErr:       gateErr,
		Confirmation:      confirmation,
		TransportApproved: isApprovedLiveSubmitTransport(ex, cfg),
	})
	logPreSubmitPolicyResult(logger, policyResult, accountID, idemKey, gateView)
	if !policyResult.Allowed {
		reasons := make([]string, 0, len(policyResult.DenyReasons))
		for _, reason := range policyResult.DenyReasons {
			reasons = append(reasons, string(reason))
		}
		return fmt.Errorf("submit blocked by pre-submit policy: %s", strings.Join(reasons, ","))
	}

	identity, err := deriveSubmitIdentity(accountID, idemKey, confirmation.OrderHash)
	if err != nil {
		return fmt.Errorf("submit blocked by duplicate-submit policy: %s", DuplicateSubmitUnknownState)
	}
	dupResult := liveSubmitIdentities.reserve(identity)
	logDuplicateSubmitCheck(logger, identity, dupResult)
	if !dupResult.Allowed {
		return fmt.Errorf("submit blocked by duplicate-submit policy: %s", dupResult.DenyReason)
	}

	writeOrderIntent("submit", accountID, order, idemKey)

	if !flagJSON {
		fmt.Println("Proceeding with live submission...")
	}
	result, err := ex.Submit(ctx, accountID, order, idemKey)
	if err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	markResult := liveSubmitIdentities.markSubmitted(identity)
	logDuplicateSubmitCheck(logger, identity, markResult)
	if !markResult.Allowed {
		return fmt.Errorf("submit blocked by duplicate-submit policy: %s", markResult.DenyReason)
	}
	iclient.Metrics.OrdersSubmitted.WithLabelValues("submit").Inc()

	if flagJSON {
		out := SubmitOutput{
			Submitted:          true,
			Warnings:           make([]DryRunErrorOut, 0, len(result.Warnings)),
			BuyingPowerEffect:  buildBPEffectOut(result.BuyingPowerEffect),
			Order:              buildOrderSummary(result.Order),
			DecisionGateStatus: string(gateView.Decision.Outcome),
		}
		for _, w := range result.Warnings {
			out.Warnings = append(out.Warnings, DryRunErrorOut{Code: w.Code, Message: w.Message})
		}
		return printJSON(out)
	}

	fmt.Println("✓ ORDER SUBMITTED")
	fmt.Printf("  Order ID:      %s\n", result.Order.ID)
	fmt.Printf("  Status:        %s\n", result.Order.Status)
	fmt.Printf("  Type:          %s\n", result.Order.OrderType)
	fmt.Printf("  Time In Force: %s\n", result.Order.TimeInForce)
	bp := result.BuyingPowerEffect
	fmt.Printf("  BP Change:     %s %s\n", bp.ChangeInBuyingPower, bp.ChangeInBuyingPowerEffect)
	fmt.Printf("  Margin Change: %s %s\n", bp.ChangeInMarginRequirement, bp.ChangeInMarginRequirementEffect)
	fmt.Printf("  Current BP:    %s\n", bp.CurrentBuyingPower)
	fmt.Printf("  New BP:        %s\n", bp.NewBuyingPower)
	if len(result.Warnings) > 0 {
		fmt.Printf("  Warnings (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("    [%s] %s\n", w.Code, w.Message)
		}
	}
	return nil
}

// ── Dry-run command ──────────────────────────────────────────────────────────

var dryRunCmd = &cobra.Command{
	Use:   "dry-run",
	Short: "Simulate an order without submitting (always safe)",
	Long: `Submits an order to /orders/dry-run for validation.

Never submits a live order. Use --json for stable output for the automation pipeline.

The order payload must be passed as a JSON file:
  tt dry-run --file order.json [--json]

Example order.json (iron condor):
  {
    "order-type": "Limit",
    "time-in-force": "Day",
    "price": "1.20",
    "price-effect": "Credit",
    "legs": [
      {"instrument-type": "Equity Option", "symbol": ".XSP250117C580", "quantity": 1, "action": "Sell to Open"},
      {"instrument-type": "Equity Option", "symbol": ".XSP250117C590", "quantity": 1, "action": "Buy to Open"},
      {"instrument-type": "Equity Option", "symbol": ".XSP250117P520", "quantity": 1, "action": "Sell to Open"},
      {"instrument-type": "Equity Option", "symbol": ".XSP250117P510", "quantity": 1, "action": "Buy to Open"}
    ]
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDryRun(cmd.Context())
	},
}

// DryRunOutput is the stable --json schema for automation pipeline consumers.
type DryRunOutput struct {
	OK                bool             `json:"ok"`       // true if no errors
	Errors            []DryRunErrorOut `json:"errors"`   // empty slice if none
	Warnings          []DryRunErrorOut `json:"warnings"` // empty slice if none
	BuyingPowerEffect BPEffectOut      `json:"buying_power_effect"`
	Order             OrderSummary     `json:"order"`
}

type DryRunErrorOut struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type BPEffectOut struct {
	ChangeInBuyingPower       string `json:"change_in_buying_power"`
	ChangeInBuyingPowerEffect string `json:"change_in_buying_power_effect"`
	ChangeInMarginReq         string `json:"change_in_margin_requirement"`
	ChangeInMarginReqEffect   string `json:"change_in_margin_requirement_effect"`
	CurrentBuyingPower        string `json:"current_buying_power"`
	NewBuyingPower            string `json:"new_buying_power"`
}

func runDryRun(ctx context.Context) error {
	accountID := cfg.AccountID
	if accountID == "" {
		return fmt.Errorf("dry-run: TASTYTRADE_ACCOUNT_ID is not set")
	}

	// Safety gate — spec §5: kill switch → circuit breaker → NLQ guard (stub).
	// Dry-run is gated because it counts against the orders-family rate limiter
	// and we want the same safety path as live submission.
	if err := cl.CheckOrderSafety(); err != nil {
		return fmt.Errorf("dry-run blocked: %w", err)
	}

	// Confidence gate — Phase 3B.
	// Dry-run is the current confidence-dependent order decision entry point.
	// We keep the existing order-safety checks intact, then consult the latest
	// reconciler policy before consuming the orders-family endpoint.
	if err := enforceDecisionGate("dry-run", rec, logger); err != nil {
		return err
	}

	order, err := parseOrderFile(flagDryRunFile, "dry-run")
	if err != nil {
		return err
	}
	idemKey := uuid.NewString()
	writeOrderIntent("dry-run", accountID, order, idemKey)

	// Execute via the exchange layer, passing the pre-logged idempotency key
	// so the intent log entry and X-Idempotency-Key header carry the same UUID.
	result, err := ex.DryRun(ctx, accountID, order, idemKey)
	if err != nil {
		return fmt.Errorf("dry-run: %w", err)
	}

	if flagJSON {
		out := DryRunOutput{
			OK:                len(result.Errors) == 0,
			Errors:            make([]DryRunErrorOut, 0, len(result.Errors)),
			Warnings:          make([]DryRunErrorOut, 0, len(result.Warnings)),
			BuyingPowerEffect: buildBPEffectOut(result.BuyingPowerEffect),
			Order:             buildOrderSummary(result.Order),
		}
		for _, e := range result.Errors {
			out.Errors = append(out.Errors, DryRunErrorOut{Code: e.Code, Message: e.Message})
		}
		for _, w := range result.Warnings {
			out.Warnings = append(out.Warnings, DryRunErrorOut{Code: w.Code, Message: w.Message})
		}
		return printJSON(out)
	}

	// Human-readable
	if len(result.Errors) > 0 {
		fmt.Println("✗ DRY-RUN REJECTED:")
		for _, e := range result.Errors {
			fmt.Printf("  [%s] %s\n", e.Code, e.Message)
		}
		return fmt.Errorf("dry-run: order rejected (%d errors)", len(result.Errors))
	}
	fmt.Println("✓ DRY-RUN OK")
	bp := result.BuyingPowerEffect
	fmt.Printf("  BP Change:     %s %s\n", bp.ChangeInBuyingPower, bp.ChangeInBuyingPowerEffect)
	fmt.Printf("  Margin Change: %s %s\n", bp.ChangeInMarginRequirement, bp.ChangeInMarginRequirementEffect)
	fmt.Printf("  Current BP:    %s\n", bp.CurrentBuyingPower)
	fmt.Printf("  New BP:        %s\n", bp.NewBuyingPower)
	if len(result.Warnings) > 0 {
		fmt.Printf("  Warnings (%d):\n", len(result.Warnings))
		for _, w := range result.Warnings {
			fmt.Printf("    [%s] %s\n", w.Code, w.Message)
		}
	}
	return nil
}

func confirmLiveSubmit(accountID string, order models.NewOrder, intentID string) (*SubmitConfirmation, error) {
	orderHash, err := canonicalOrderHash(order)
	if err != nil {
		return nil, fmt.Errorf("submit confirmation failed: %w", err)
	}
	confirmation := &SubmitConfirmation{
		Action:    "submit",
		AccountID: accountID,
		IntentID:  intentID,
		OrderHash: orderHash,
	}
	if flagJSON {
		if !flagSubmitYes {
			return nil, fmt.Errorf("submit blocked: --json mode requires --yes to acknowledge live order submission")
		}
		confirmation.Acknowledged = true
		confirmation.NonInteractive = true
		return confirmation, nil
	}

	fmt.Println("LIVE ORDER SUBMISSION")
	fmt.Println("This will submit a live order to tastytrade.")
	fmt.Printf("  Account:       %s\n", accountID)
	fmt.Printf("  Intent ID:     %s\n", intentID)
	fmt.Printf("  Type:          %s\n", order.OrderType)
	fmt.Printf("  Time In Force: %s\n", order.TimeInForce)
	if order.Price != "" || order.PriceEffect != "" {
		fmt.Printf("  Price:         %s %s\n", order.Price, order.PriceEffect)
	}
	fmt.Printf("  Legs:          %d\n", len(order.Legs))
	for i, leg := range order.Legs {
		fmt.Printf("    %d. %s %s x%d (%s)\n", i+1, leg.Action, leg.Symbol, leg.Quantity, leg.InstrumentType)
	}
	fmt.Print("Type 'submit' to confirm live order submission: ")

	reader := bufio.NewReader(submitConfirmIn)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("submit confirmation failed: %w", err)
	}
	if strings.TrimSpace(strings.ToLower(line)) != "submit" {
		fmt.Println("submit declined by operator")
		return nil, fmt.Errorf("submit aborted: operator declined confirmation")
	}
	confirmation.Acknowledged = true
	return confirmation, nil
}

func buildOrderSummary(o models.Order) OrderSummary {
	os := OrderSummary{
		ID:          o.ID,
		Status:      o.Status,
		OrderType:   o.OrderType,
		TimeInForce: o.TimeInForce,
		Price:       o.Price,
		PriceEffect: o.PriceEffect,
	}
	if !o.ReceivedAt.IsZero() {
		os.ReceivedAt = o.ReceivedAt.Format("2006-01-02T15:04:05Z")
	}
	for _, l := range o.Legs {
		os.Legs = append(os.Legs, LegSummary{
			Symbol:         l.Symbol,
			InstrumentType: l.InstrumentType,
			Action:         l.Action,
			Quantity:       l.Quantity.String(),
		})
	}
	return os
}

func buildBPEffectOut(bp models.BPEffect) BPEffectOut {
	return BPEffectOut{
		ChangeInBuyingPower:       bp.ChangeInBuyingPower.String(),
		ChangeInBuyingPowerEffect: bp.ChangeInBuyingPowerEffect,
		ChangeInMarginReq:         bp.ChangeInMarginRequirement.String(),
		ChangeInMarginReqEffect:   bp.ChangeInMarginRequirementEffect,
		CurrentBuyingPower:        bp.CurrentBuyingPower.String(),
		NewBuyingPower:            bp.NewBuyingPower.String(),
	}
}

func parseOrderFile(path, action string) (models.NewOrder, error) {
	orderData, err := readFile(path)
	if err != nil {
		return models.NewOrder{}, fmt.Errorf("%s: read order file: %w", action, err)
	}
	var order models.NewOrder
	if err := json.Unmarshal(orderData, &order); err != nil {
		return models.NewOrder{}, fmt.Errorf("%s: parse order file: %w", action, err)
	}
	return order, nil
}

func writeOrderIntent(strategy, accountID string, order models.NewOrder, idemKey string) {
	firstSymbol, firstQty := "", ""
	if len(order.Legs) > 0 {
		firstSymbol = order.Legs[0].Symbol
		firstQty = fmt.Sprintf("%d", order.Legs[0].Quantity)
	}
	intentlog.Write(intentlog.Entry{
		AccountID:      accountID,
		Symbol:         firstSymbol,
		Strategy:       strategy,
		Quantity:       firstQty,
		Price:          order.Price,
		PriceEffect:    order.PriceEffect,
		OrderType:      order.OrderType,
		TimeInForce:    order.TimeInForce,
		LegCount:       len(order.Legs),
		IdempotencyKey: idemKey,
	}, logger)
}

// readFile is a thin wrapper to allow test injection.
var readFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
