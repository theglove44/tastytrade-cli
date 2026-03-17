package cmd

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
)

var positionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "List open positions",
	Long: `List open positions for the configured account.

Use --json to emit stable JSON for the automation pipeline.
The JSON schema is stable — field names will not change without a version bump.

Example pipeline usage:
  tt positions --json | jq '.positions[] | select(.underlying=="XSP")'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPositions(cmd.Context())
	},
}

// PositionsOutput is the stable --json schema for automation pipeline consumers.
// This struct is the contract — do not rename fields without a version bump.
type PositionsOutput struct {
	AccountNumber string            `json:"account_number"`
	Positions     []PositionSummary `json:"positions"`
	Count         int               `json:"count"`
}

type PositionSummary struct {
	Symbol            string          `json:"symbol"`
	UnderlyingSymbol  string          `json:"underlying_symbol"`
	InstrumentType    string          `json:"instrument_type"`
	Quantity          decimal.Decimal `json:"quantity"`
	QuantityDirection string          `json:"quantity_direction"` // Long | Short
	AverageOpenPrice  decimal.Decimal `json:"average_open_price"`
	ClosePrice        decimal.Decimal `json:"close_price"`
	// UnrealizedPnL is close_price - avg_open * quantity * direction_sign.
	// Provided as a convenience; recalculate with live quotes for accuracy.
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`
	ExpiresAt     string          `json:"expires_at,omitempty"` // RFC3339 or ""
}

func runPositions(ctx context.Context) error {
	accountID, err := resolveAccountID(ctx, "positions")
	if err != nil {
		return err
	}

	items, err := ex.Positions(ctx, accountID)
	if err != nil {
		return fmt.Errorf("positions: %w", err)
	}

	if flagJSON {
		out := PositionsOutput{
			AccountNumber: accountID,
			Count:         len(items),
		}
		for _, p := range items {
			ps := PositionSummary{
				Symbol:            p.Symbol,
				UnderlyingSymbol:  p.UnderlyingSymbol,
				InstrumentType:    p.InstrumentType,
				Quantity:          p.Quantity,
				QuantityDirection: p.QuantityDirection,
				AverageOpenPrice:  p.AverageOpenPrice,
				ClosePrice:        p.ClosePrice,
			}
			// Unrealised P&L estimate (Long: close - avg_open; Short: avg_open - close)
			if p.QuantityDirection == "Short" {
				ps.UnrealizedPnL = p.AverageOpenPrice.Sub(p.ClosePrice).Mul(p.Quantity)
			} else {
				ps.UnrealizedPnL = p.ClosePrice.Sub(p.AverageOpenPrice).Mul(p.Quantity)
			}
			if p.ExpiresAt != nil {
				ps.ExpiresAt = p.ExpiresAt.Format("2006-01-02T15:04:05Z")
			}
			out.Positions = append(out.Positions, ps)
		}
		return printJSON(out)
	}

	// Human-readable table
	if len(items) == 0 {
		fmt.Println("No open positions.")
		return nil
	}
	fmt.Printf("%-28s %-16s %-8s %-8s %-14s %-14s\n",
		"SYMBOL", "TYPE", "QTY", "DIR", "AVG OPEN", "CLOSE")
	for _, p := range items {
		fmt.Printf("%-28s %-16s %-8s %-8s %-14s %-14s\n",
			p.Symbol, p.InstrumentType,
			p.Quantity.String(), p.QuantityDirection,
			p.AverageOpenPrice.String(), p.ClosePrice.String())
	}
	fmt.Printf("\nTotal positions: %d\n", len(items))
	return nil
}
