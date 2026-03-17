package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

var (
	flagBrokerOrdersLimit int
	flagBrokerOrderID     string

	likelyLocalSubmitIdentityPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

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

var brokerOrdersDetailCmd = &cobra.Command{
	Use:   "detail",
	Short: "Inspect one broker order in detail by broker order ID",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBrokerOrdersDetail(cmd.Context())
	},
}

func init() {
	brokerOrdersRecentCmd.Flags().IntVar(&flagBrokerOrdersLimit, "limit", 10, "Maximum recent broker orders to return")
	brokerOrdersDetailCmd.Flags().StringVar(&flagBrokerOrderID, "id", "", "Canonical broker order ID to inspect")
	_ = brokerOrdersDetailCmd.MarkFlagRequired("id")
	brokerOrdersCmd.AddCommand(brokerOrdersLiveCmd, brokerOrdersRecentCmd, brokerOrdersDetailCmd)
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

type BrokerOrderDetailOutput struct {
	AccountNumber string          `json:"account_number"`
	Order         BrokerOrderView `json:"order"`
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

func runBrokerOrdersDetail(ctx context.Context) error {
	accountID, err := resolveAccountID(ctx, "broker-orders detail")
	if err != nil {
		return err
	}
	if err := validateBrokerOrderID(flagBrokerOrderID); err != nil {
		return fmt.Errorf("broker-orders detail: %w", err)
	}
	order, err := ex.Order(ctx, accountID, flagBrokerOrderID)
	if err != nil {
		return fmt.Errorf("broker-orders detail: %w", err)
	}
	out := BrokerOrderDetailOutput{AccountNumber: accountID, Order: buildBrokerOrderView(order)}
	if flagJSON {
		return printJSON(out)
	}
	return renderBrokerOrderDetail(accountID, order)
}

func validateBrokerOrderID(orderID string) error {
	trimmed := strings.TrimSpace(orderID)
	if trimmed == "" {
		return fmt.Errorf("canonical broker order id is required")
	}
	if likelyLocalSubmitIdentityPattern.MatchString(strings.ToLower(trimmed)) {
		return fmt.Errorf("canonical broker order id required; got a value that looks like a local submit identity")
	}
	return nil
}

func renderBrokerOrderDetail(accountID string, order models.Order) error {
	out := BrokerOrderDetailOutput{AccountNumber: accountID, Order: buildBrokerOrderView(order)}
	fmt.Println("BROKER ORDER DETAIL")
	fmt.Println("  order:")
	fmt.Printf("    account=%s\n", out.AccountNumber)
	fmt.Printf("    id=%s\n", out.Order.ID)
	fmt.Printf("    status=%s\n", out.Order.Status)
	fmt.Printf("    type=%s\n", out.Order.OrderType)
	fmt.Printf("    time_in_force=%s\n", out.Order.TimeInForce)
	if out.Order.Price != "" || out.Order.PriceEffect != "" {
		fmt.Println("  pricing:")
		if out.Order.Price != "" {
			fmt.Printf("    price=%s\n", out.Order.Price)
		}
		if out.Order.PriceEffect != "" {
			fmt.Printf("    price_effect=%s\n", out.Order.PriceEffect)
		}
	}
	if out.Order.ReceivedAt != "" || out.Order.UpdatedAt != "" || out.Order.FilledAt != "" || out.Order.CancelledAt != "" {
		fmt.Println("  timestamps:")
		if out.Order.ReceivedAt != "" {
			fmt.Printf("    received_at=%s\n", out.Order.ReceivedAt)
		}
		if out.Order.UpdatedAt != "" {
			fmt.Printf("    updated_at=%s\n", out.Order.UpdatedAt)
		}
		if out.Order.FilledAt != "" {
			fmt.Printf("    filled_at=%s\n", out.Order.FilledAt)
		}
		if out.Order.CancelledAt != "" {
			fmt.Printf("    cancelled_at=%s\n", out.Order.CancelledAt)
		}
	}
	fmt.Println("  legs:")
	if len(order.Legs) == 0 {
		fmt.Println("    (none)")
		return nil
	}
	for i, leg := range order.Legs {
		fmt.Printf("    %d) %s\n", i+1, leg.Symbol)
		fmt.Printf("       action=%s quantity=%s\n", leg.Action, leg.Quantity.String())
		if leg.InstrumentType != "" {
			fmt.Printf("       instrument_type=%s\n", leg.InstrumentType)
		}
		if hasLegFillContext(leg) {
			fmt.Println("       fill_context:")
			if !leg.FillQuantity.IsZero() {
				fmt.Printf("         fill_quantity=%s\n", leg.FillQuantity.String())
			}
			if !leg.FillPrice.IsZero() {
				fmt.Printf("         average_fill_price=%s\n", leg.FillPrice.String())
			}
		}
	}
	return nil
}

func hasLegFillContext(leg models.OrderLeg) bool {
	return !leg.FillQuantity.IsZero() || !leg.FillPrice.IsZero()
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
