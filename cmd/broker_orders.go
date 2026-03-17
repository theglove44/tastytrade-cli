package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

var flagBrokerOrdersLimit int

var brokerOrdersCmd = &cobra.Command{
	Use:   "broker-orders",
	Short: "Read-only broker-facing order inspection",
	Long: `Read-only broker-facing order inspection commands.

These commands only fetch tastytrade order state from the API.
They do not mutate broker orders, reconcile broker state, or affect Phase 3C submit safety behavior.`,
}

var brokerOrdersLiveCmd = &cobra.Command{
	Use:   "live",
	Short: "Inspect broker live/open order state",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBrokerOrdersLive(cmd.Context())
	},
}

var brokerOrdersRecentCmd = &cobra.Command{
	Use:   "recent",
	Short: "Inspect recent broker order state",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBrokerOrdersRecent(cmd.Context())
	},
}

func init() {
	brokerOrdersRecentCmd.Flags().IntVar(&flagBrokerOrdersLimit, "limit", 10, "Maximum recent broker orders to return")
	brokerOrdersCmd.AddCommand(brokerOrdersLiveCmd, brokerOrdersRecentCmd)
}

type BrokerOrdersOutput struct {
	AccountNumber string            `json:"account_number"`
	Source        string            `json:"source"`
	Count         int               `json:"count"`
	Orders        []BrokerOrderView `json:"orders"`
}

type BrokerOrderView struct {
	ID          string       `json:"id"`
	Status      string       `json:"status"`
	OrderType   string       `json:"order_type"`
	TimeInForce string       `json:"time_in_force"`
	Price       string       `json:"price"`
	PriceEffect string       `json:"price_effect"`
	ReceivedAt  string       `json:"received_at,omitempty"`
	UpdatedAt   string       `json:"updated_at,omitempty"`
	FilledAt    string       `json:"filled_at,omitempty"`
	CancelledAt string       `json:"cancelled_at,omitempty"`
	Legs        []LegSummary `json:"legs"`
}

func runBrokerOrdersLive(ctx context.Context) error {
	accountID, err := resolveAccountID(ctx, "broker-orders live")
	if err != nil {
		return err
	}
	items, err := ex.Orders(ctx, accountID)
	if err != nil {
		return fmt.Errorf("broker-orders live: %w", err)
	}
	return renderBrokerOrders(accountID, "live", items)
}

func runBrokerOrdersRecent(ctx context.Context) error {
	accountID, err := resolveAccountID(ctx, "broker-orders recent")
	if err != nil {
		return err
	}
	limit := flagBrokerOrdersLimit
	if limit <= 0 {
		limit = 10
	}
	items, err := ex.RecentOrders(ctx, accountID, limit)
	if err != nil {
		return fmt.Errorf("broker-orders recent: %w", err)
	}
	return renderBrokerOrders(accountID, fmt.Sprintf("recent(limit=%d)", limit), items)
}

func renderBrokerOrders(accountID, source string, items []models.Order) error {
	out := BrokerOrdersOutput{AccountNumber: accountID, Source: source, Count: len(items)}
	for _, item := range items {
		out.Orders = append(out.Orders, buildBrokerOrderView(item))
	}
	if flagJSON {
		return printJSON(out)
	}
	if len(out.Orders) == 0 {
		fmt.Printf("No broker orders returned for %s.\n", source)
		return nil
	}
	fmt.Printf("BROKER ORDERS (%s)\n", source)
	fmt.Printf("%-12s %-12s %-10s %-20s %s\n", "ID", "STATUS", "TYPE", "UPDATED", "LEGS")
	for _, order := range out.Orders {
		legs := make([]string, len(order.Legs))
		for i, leg := range order.Legs {
			legs[i] = fmt.Sprintf("%s %s x%s", leg.Action, leg.Symbol, leg.Quantity)
		}
		updated := order.UpdatedAt
		if updated == "" {
			updated = order.ReceivedAt
		}
		fmt.Printf("%-12s %-12s %-10s %-20s %s\n",
			order.ID,
			order.Status,
			order.OrderType,
			updated,
			strings.Join(legs, " | "),
		)
	}
	return nil
}

func buildBrokerOrderView(order models.Order) BrokerOrderView {
	view := BrokerOrderView{
		ID:          order.ID,
		Status:      order.Status,
		OrderType:   order.OrderType,
		TimeInForce: order.TimeInForce,
		Price:       order.Price.String(),
		PriceEffect: order.PriceEffect,
	}
	if !order.ReceivedAt.IsZero() {
		view.ReceivedAt = order.ReceivedAt.UTC().Format(time.RFC3339)
	}
	if !order.UpdatedAt.IsZero() {
		view.UpdatedAt = order.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if order.FilledAt != nil {
		view.FilledAt = order.FilledAt.UTC().Format(time.RFC3339)
	}
	if order.CancelledAt != nil {
		view.CancelledAt = order.CancelledAt.UTC().Format(time.RFC3339)
	}
	for _, leg := range order.Legs {
		view.Legs = append(view.Legs, LegSummary{
			Symbol:         leg.Symbol,
			InstrumentType: leg.InstrumentType,
			Action:         leg.Action,
			Quantity:       leg.Quantity.String(),
		})
	}
	return view
}
