package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
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
			os := OrderSummary{
				ID:          o.ID,
				Status:      o.Status,
				OrderType:   o.OrderType,
				TimeInForce: o.TimeInForce,
				Price:       o.Price,
				PriceEffect: o.PriceEffect,
				ReceivedAt:  o.ReceivedAt.Format("2006-01-02T15:04:05Z"),
			}
			for _, l := range o.Legs {
				os.Legs = append(os.Legs, LegSummary{
					Symbol:         l.Symbol,
					InstrumentType: l.InstrumentType,
					Action:         l.Action,
					Quantity:       l.Quantity.String(),
				})
			}
			out.Orders = append(out.Orders, os)
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

// ── Dry-run command ──────────────────────────────────────────────────────────

var (
	flagDryRunFile string // path to JSON file with NewOrder payload
)

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

func init() {
	dryRunCmd.Flags().StringVar(&flagDryRunFile, "file", "",
		"Path to JSON file containing the NewOrder payload (required)")
	_ = dryRunCmd.MarkFlagRequired("file")
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

	// Read and parse the order payload.
	orderData, err := readFile(flagDryRunFile)
	if err != nil {
		return fmt.Errorf("dry-run: read order file: %w", err)
	}
	var order models.NewOrder
	if err := json.Unmarshal(orderData, &order); err != nil {
		return fmt.Errorf("dry-run: parse order file: %w", err)
	}

	// Generate idempotency key before dispatch and record in intent log.
	// The same key is used for the HTTP request via the exchange layer.
	idemKey := uuid.NewString()
	firstSymbol, firstQty := "", ""
	if len(order.Legs) > 0 {
		firstSymbol = order.Legs[0].Symbol
		firstQty = fmt.Sprintf("%d", order.Legs[0].Quantity)
	}
	intentlog.Write(intentlog.Entry{
		AccountID:      accountID,
		Symbol:         firstSymbol,
		Strategy:       "dry-run",
		Quantity:       firstQty,
		Price:          order.Price,
		PriceEffect:    order.PriceEffect,
		OrderType:      order.OrderType,
		TimeInForce:    order.TimeInForce,
		LegCount:       len(order.Legs),
		IdempotencyKey: idemKey,
	}, logger)

	// Execute via the exchange layer, passing the pre-logged idempotency key
	// so the intent log entry and X-Idempotency-Key header carry the same UUID.
	result, err := ex.DryRun(ctx, accountID, order, idemKey)
	if err != nil {
		return fmt.Errorf("dry-run: %w", err)
	}

	if flagJSON {
		out := DryRunOutput{
			OK:       len(result.Errors) == 0,
			Errors:   make([]DryRunErrorOut, 0, len(result.Errors)),
			Warnings: make([]DryRunErrorOut, 0, len(result.Warnings)),
			BuyingPowerEffect: BPEffectOut{
				ChangeInBuyingPower:       result.BuyingPowerEffect.ChangeInBuyingPower.String(),
				ChangeInBuyingPowerEffect: result.BuyingPowerEffect.ChangeInBuyingPowerEffect,
				ChangeInMarginReq:         result.BuyingPowerEffect.ChangeInMarginRequirement.String(),
				ChangeInMarginReqEffect:   result.BuyingPowerEffect.ChangeInMarginRequirementEffect,
				CurrentBuyingPower:        result.BuyingPowerEffect.CurrentBuyingPower.String(),
				NewBuyingPower:            result.BuyingPowerEffect.NewBuyingPower.String(),
			},
		}
		for _, e := range result.Errors {
			out.Errors = append(out.Errors, DryRunErrorOut{Code: e.Code, Message: e.Message})
		}
		for _, w := range result.Warnings {
			out.Warnings = append(out.Warnings, DryRunErrorOut{Code: w.Code, Message: w.Message})
		}
		o := result.Order
		os := OrderSummary{
			ID:          o.ID,
			Status:      o.Status,
			OrderType:   o.OrderType,
			TimeInForce: o.TimeInForce,
			Price:       o.Price,
			PriceEffect: o.PriceEffect,
		}
		for _, l := range o.Legs {
			os.Legs = append(os.Legs, LegSummary{
				Symbol:         l.Symbol,
				InstrumentType: l.InstrumentType,
				Action:         l.Action,
				Quantity:       l.Quantity.String(),
			})
		}
		out.Order = os
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

// readFile is a thin wrapper to allow test injection.
var readFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
