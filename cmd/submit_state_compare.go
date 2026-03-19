package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

var (
	flagSubmitStateCompareLimit   int
	flagSubmitStateCompareAccount string
	flagSubmitStateCompareOutcome string
)

const (
	ComparisonPlausibleMatch     = "local_present_broker_match"
	ComparisonLocalNoBrokerMatch = "local_uncertain_no_broker_match"
	ComparisonBrokerNoLocalState = "broker_order_no_local_state"
	ComparisonAmbiguous          = "ambiguous"
	comparisonAdvisoryDisclaimer = "advisory_manual_only"
)

// SubmitStateCompareOutput is the stable --json schema for local vs broker comparison.
type SubmitStateCompareOutput struct {
	Advisory      string                      `json:"advisory"`
	AccountNumber string                      `json:"account_number"`
	BrokerSource  string                      `json:"broker_source"`
	OutcomeFilter string                      `json:"outcome_filter,omitempty"`
	LocalCount    int                         `json:"local_count"`
	BrokerCount   int                         `json:"broker_count"`
	Summary       []SubmitStateCompareSummary `json:"summary"`
	Results       []SubmitStateCompareEntry   `json:"results"`
}

type SubmitStateCompareSummary struct {
	Outcome string `json:"outcome"`
	Count   int    `json:"count"`
}

type SubmitStateCompareEntry struct {
	Outcome            string   `json:"outcome"`
	SubmitIdentity     string   `json:"submit_identity,omitempty"`
	LocalState         string   `json:"local_state,omitempty"`
	OrderHash          string   `json:"order_hash,omitempty"`
	BrokerOrderID      string   `json:"broker_order_id,omitempty"`
	BrokerStatus       string   `json:"broker_status,omitempty"`
	Note               string   `json:"note"`
	RecommendedActions []string `json:"recommended_actions,omitempty"`
}

var submitStateCompareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Advisory local vs broker order comparison with summary/filter support",
	Long: `Advisory local vs broker order comparison for manual troubleshooting.

This command compares local persisted submit safety state with broker-visible order state,
then surfaces concise summary counts, optional advisory filters, and recommended next actions.
Use it as part of a manual reconciliation workflow together with submit-state inspect
and broker-orders live/recent.
It is read-only only: no reconciliation, no local state mutation, and no broker mutation.
Comparison results are advisory/manual only and cannot confirm broker truth conclusively.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmitStateCompare(cmd.Context())
	},
}

func init() {
	submitStateCompareCmd.Flags().IntVar(&flagSubmitStateCompareLimit, "limit", 25, "Maximum recent broker orders to include in comparison")
	submitStateCompareCmd.Flags().StringVar(&flagSubmitStateCompareAccount, "account", "", "Account ID to compare (overrides configured/default account selection)")
	submitStateCompareCmd.Flags().StringVar(&flagSubmitStateCompareOutcome, "outcome", "", "Optional outcome filter: local_present_broker_match, local_uncertain_no_broker_match, broker_order_no_local_state, ambiguous")
	submitStateCmd.AddCommand(submitStateCompareCmd)
}

func runSubmitStateCompare(ctx context.Context) error {
	accountID, err := resolveSubmitStateCompareAccountID(ctx)
	if err != nil {
		return err
	}
	if err := validateSubmitStateCompareOutcomeFilter(flagSubmitStateCompareOutcome); err != nil {
		return err
	}

	localRecords, denyReason, err := liveSubmitIdentities.inspect()
	if err != nil {
		if flagJSON {
			return printJSON(map[string]any{
				"status":         "deny",
				"advisory":       comparisonAdvisoryDisclaimer,
				"account_number": accountID,
				"deny_reason":    denyReason,
			})
		}
		fmt.Println("LOCAL VS BROKER ORDER COMPARISON DENIED")
		fmt.Printf("  status=deny primary_reason=%s\n", denyReason)
		fmt.Println("  local persisted submit state is invalid or ambiguous; manual inspection is required")
		return fmt.Errorf("submit-state compare denied: %s", denyReason)
	}

	limit := flagSubmitStateCompareLimit
	if limit <= 0 {
		limit = 25
	}
	liveOrders, err := ex.Orders(ctx, accountID)
	if err != nil {
		return fmt.Errorf("submit-state compare: live broker orders: %w", err)
	}
	recentOrders, err := ex.RecentOrders(ctx, accountID, limit)
	if err != nil {
		return fmt.Errorf("submit-state compare: recent broker orders: %w", err)
	}

	results, brokerOrders := compareLocalSubmitStateToBroker(accountID, localRecords, liveOrders, recentOrders)
	results = filterSubmitStateCompareResultsByOutcome(results, flagSubmitStateCompareOutcome)
	summary := summarizeSubmitStateCompareResults(results)
	out := SubmitStateCompareOutput{
		Advisory:      comparisonAdvisoryDisclaimer,
		AccountNumber: accountID,
		BrokerSource:  fmt.Sprintf("live+recent(limit=%d)", limit),
		OutcomeFilter: flagSubmitStateCompareOutcome,
		LocalCount:    countRecordsForAccount(localRecords, accountID),
		BrokerCount:   len(brokerOrders),
		Summary:       summary,
		Results:       results,
	}
	if flagJSON {
		return printJSON(out)
	}
	fmt.Println("LOCAL VS BROKER ORDER COMPARISON")
	fmt.Println("  advisory=manual_only")
	fmt.Printf("  account=%s broker_source=%s local=%d broker=%d\n", out.AccountNumber, out.BrokerSource, out.LocalCount, out.BrokerCount)
	if out.OutcomeFilter != "" {
		fmt.Printf("  outcome_filter=%s\n", out.OutcomeFilter)
	}
	fmt.Println("  summary:")
	for _, item := range out.Summary {
		fmt.Printf("    %s=%d\n", item.Outcome, item.Count)
	}
	if len(out.Results) == 0 {
		fmt.Println("  no comparison results for the selected account")
		fmt.Println("  comparison is advisory only and does not confirm broker truth")
		return nil
	}
	for _, result := range out.Results {
		fmt.Printf("- outcome=%s", result.Outcome)
		if result.SubmitIdentity != "" {
			fmt.Printf(" submit_identity=%s", result.SubmitIdentity)
		}
		if result.LocalState != "" {
			fmt.Printf(" local_state=%s", result.LocalState)
		}
		if result.BrokerOrderID != "" {
			fmt.Printf(" broker_order_id=%s", result.BrokerOrderID)
		}
		if result.BrokerStatus != "" {
			fmt.Printf(" broker_status=%s", result.BrokerStatus)
		}
		fmt.Println()
		fmt.Printf("  note=%s\n", result.Note)
		for _, action := range result.RecommendedActions {
			fmt.Printf("  next_action=%s\n", action)
		}
		for _, hint := range comparisonNextStepHints(result, limit) {
			fmt.Printf("  next_step=%s\n", hint)
		}
	}
	fmt.Println("  comparison is advisory/manual only; no reconciliation or broker confirmation is performed")
	return nil
}

func compareLocalSubmitStateToBroker(accountID string, local []SubmitStateRecordView, liveOrders, recentOrders []models.Order) ([]SubmitStateCompareEntry, []models.Order) {
	local = filterLocalRecordsByAccount(local, accountID)
	brokerOrders := mergeBrokerOrders(liveOrders, recentOrders)

	brokerByHash := map[string][]models.Order{}
	localByHash := map[string][]SubmitStateRecordView{}
	brokerHashByID := map[string]string{}

	for _, record := range local {
		localByHash[record.OrderHash] = append(localByHash[record.OrderHash], record)
	}
	for _, order := range brokerOrders {
		hash, err := brokerOrderHash(order)
		if err != nil {
			brokerHashByID[order.ID] = ""
			continue
		}
		brokerHashByID[order.ID] = hash
		brokerByHash[hash] = append(brokerByHash[hash], order)
	}

	results := make([]SubmitStateCompareEntry, 0)
	for _, record := range local {
		matches := brokerByHash[record.OrderHash]
		sameHashLocals := localByHash[record.OrderHash]
		switch {
		case len(matches) == 1 && len(sameHashLocals) == 1:
			results = append(results, withRecommendedActions(SubmitStateCompareEntry{
				Outcome:        ComparisonPlausibleMatch,
				SubmitIdentity: record.SubmitIdentity,
				LocalState:     record.State,
				OrderHash:      record.OrderHash,
				BrokerOrderID:  matches[0].ID,
				BrokerStatus:   matches[0].Status,
				Note:           "exact local order_hash matched one broker-visible order; plausible match only",
			}))
		case len(matches) == 0 && record.State == string(SubmitIdentityInFlight):
			results = append(results, withRecommendedActions(SubmitStateCompareEntry{
				Outcome:        ComparisonLocalNoBrokerMatch,
				SubmitIdentity: record.SubmitIdentity,
				LocalState:     record.State,
				OrderHash:      record.OrderHash,
				Note:           "local state remains in_flight but no exact broker-visible match was found in current broker inspection scope",
			}))
		default:
			results = append(results, withRecommendedActions(SubmitStateCompareEntry{
				Outcome:        ComparisonAmbiguous,
				SubmitIdentity: record.SubmitIdentity,
				LocalState:     record.State,
				OrderHash:      record.OrderHash,
				Note:           buildLocalAmbiguousNote(record, matches, sameHashLocals),
			}))
		}
	}

	for _, order := range brokerOrders {
		hash := brokerHashByID[order.ID]
		if hash == "" {
			results = append(results, withRecommendedActions(SubmitStateCompareEntry{
				Outcome:       ComparisonAmbiguous,
				BrokerOrderID: order.ID,
				BrokerStatus:  order.Status,
				Note:          "broker order could not be converted into a comparable local fingerprint",
			}))
			continue
		}
		if len(localByHash[hash]) == 0 {
			results = append(results, withRecommendedActions(SubmitStateCompareEntry{
				Outcome:       ComparisonBrokerNoLocalState,
				BrokerOrderID: order.ID,
				BrokerStatus:  order.Status,
				OrderHash:     hash,
				Note:          "broker-visible order had no exact local persisted order_hash match for the selected account",
			}))
		}
	}

	sortSubmitStateCompareEntries(results)
	return results, brokerOrders
}

func recommendedActionsForOutcome(outcome string) []string {
	switch outcome {
	case ComparisonPlausibleMatch:
		return []string{
			"inspect the broker order details and current status manually",
			"keep local submit safety state until manual verification is complete",
			"only use submit-state clear after manual verification if local safety reset is still needed",
		}
	case ComparisonLocalNoBrokerMatch:
		return []string{
			"re-check broker visibility using broker-orders live and broker-orders recent",
			"treat local state as uncertain until manual verification is complete",
			"do not retry or clear local state automatically",
		}
	case ComparisonBrokerNoLocalState:
		return []string{
			"inspect the broker order details and account activity manually",
			"confirm whether the broker-visible order is expected before taking any local action",
			"do not infer that local safety state should be created or cleared automatically",
		}
	case ComparisonAmbiguous:
		return []string{
			"inspect local submit-state records and broker order details manually",
			"narrow the comparison using account, outcome, and limit filters if helpful",
			"do not clear local state or assume broker truth from an ambiguous result",
		}
	default:
		return nil
	}
}

func withRecommendedActions(entry SubmitStateCompareEntry) SubmitStateCompareEntry {
	entry.RecommendedActions = recommendedActionsForOutcome(entry.Outcome)
	return entry
}

func brokerOrderDetailHint(orderID string) string {
	if strings.TrimSpace(orderID) == "" {
		return ""
	}
	return fmt.Sprintf("tt broker-orders detail --id %s", orderID)
}

func brokerOrderReinspectionHints(outcome string, limit int) []string {
	if outcome != ComparisonLocalNoBrokerMatch {
		return nil
	}
	if limit <= 0 {
		limit = 25
	}
	return []string{
		"tt broker-orders live",
		fmt.Sprintf("tt broker-orders recent --limit %d", limit),
	}
}

func comparisonNextStepHints(result SubmitStateCompareEntry, limit int) []string {
	if hint := brokerOrderDetailHint(result.BrokerOrderID); hint != "" {
		return []string{hint}
	}
	return brokerOrderReinspectionHints(result.Outcome, limit)
}

func resolveSubmitStateCompareAccountID(ctx context.Context) (string, error) {
	if strings.TrimSpace(flagSubmitStateCompareAccount) != "" {
		return strings.TrimSpace(flagSubmitStateCompareAccount), nil
	}
	return resolveAccountID(ctx, "submit-state compare")
}

func validateSubmitStateCompareOutcomeFilter(outcome string) error {
	switch strings.TrimSpace(outcome) {
	case "", ComparisonPlausibleMatch, ComparisonLocalNoBrokerMatch, ComparisonBrokerNoLocalState, ComparisonAmbiguous:
		return nil
	default:
		return fmt.Errorf("submit-state compare: unsupported outcome filter %q", outcome)
	}
}

func filterSubmitStateCompareResultsByOutcome(results []SubmitStateCompareEntry, outcome string) []SubmitStateCompareEntry {
	if strings.TrimSpace(outcome) == "" {
		return results
	}
	filtered := make([]SubmitStateCompareEntry, 0, len(results))
	for _, result := range results {
		if result.Outcome == outcome {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func summarizeSubmitStateCompareResults(results []SubmitStateCompareEntry) []SubmitStateCompareSummary {
	counts := map[string]int{
		ComparisonAmbiguous:          0,
		ComparisonBrokerNoLocalState: 0,
		ComparisonLocalNoBrokerMatch: 0,
		ComparisonPlausibleMatch:     0,
	}
	for _, result := range results {
		counts[result.Outcome]++
	}
	ordered := []string{
		ComparisonPlausibleMatch,
		ComparisonLocalNoBrokerMatch,
		ComparisonBrokerNoLocalState,
		ComparisonAmbiguous,
	}
	summary := make([]SubmitStateCompareSummary, 0, len(ordered))
	for _, outcome := range ordered {
		summary = append(summary, SubmitStateCompareSummary{Outcome: outcome, Count: counts[outcome]})
	}
	return summary
}

func filterLocalRecordsByAccount(records []SubmitStateRecordView, accountID string) []SubmitStateRecordView {
	filtered := make([]SubmitStateRecordView, 0)
	for _, record := range records {
		if record.AccountID == accountID {
			filtered = append(filtered, record)
		}
	}
	sortRecordViews(filtered)
	return filtered
}

func mergeBrokerOrders(liveOrders, recentOrders []models.Order) []models.Order {
	byID := map[string]models.Order{}
	for _, order := range append(append([]models.Order{}, liveOrders...), recentOrders...) {
		if order.ID == "" {
			continue
		}
		byID[order.ID] = order
	}
	merged := make([]models.Order, 0, len(byID))
	for _, order := range byID {
		merged = append(merged, order)
	}
	sort.Slice(merged, func(i, j int) bool {
		left := merged[i].UpdatedAt
		right := merged[j].UpdatedAt
		if !left.Equal(right) {
			return left.After(right)
		}
		return merged[i].ID < merged[j].ID
	})
	return merged
}

func brokerOrderHash(order models.Order) (string, error) {
	mapped := models.NewOrder{
		OrderType:   order.OrderType,
		TimeInForce: order.TimeInForce,
		Price:       order.Price.String(),
		PriceEffect: order.PriceEffect,
		Legs:        make([]models.NewOrderLeg, 0, len(order.Legs)),
	}
	for _, leg := range order.Legs {
		qty, err := brokerLegQuantity(leg.Quantity)
		if err != nil {
			return "", err
		}
		mapped.Legs = append(mapped.Legs, models.NewOrderLeg{
			InstrumentType: leg.InstrumentType,
			Symbol:         leg.Symbol,
			Quantity:       qty,
			Action:         leg.Action,
		})
	}
	return canonicalOrderHash(mapped)
}

func brokerLegQuantity(qty decimal.Decimal) (int, error) {
	if !qty.Equal(qty.Truncate(0)) {
		return 0, fmt.Errorf("non-integer broker quantity: %s", qty.String())
	}
	return int(qty.IntPart()), nil
}

func buildLocalAmbiguousNote(record SubmitStateRecordView, matches []models.Order, sameHashLocals []SubmitStateRecordView) string {
	parts := make([]string, 0, 3)
	if len(matches) > 1 {
		parts = append(parts, fmt.Sprintf("multiple broker-visible orders (%d) shared the same comparable fingerprint", len(matches)))
	} else if len(matches) == 0 {
		parts = append(parts, "no exact broker-visible match found")
	}
	if len(sameHashLocals) > 1 {
		parts = append(parts, fmt.Sprintf("multiple local persisted records (%d) shared the same order_hash", len(sameHashLocals)))
	}
	if record.State == string(SubmitIdentitySubmitted) && len(matches) == 0 {
		parts = append(parts, "local state is submitted, but current broker inspection scope cannot confirm a match")
	}
	if len(parts) == 0 {
		parts = append(parts, "comparison could not be classified deterministically")
	}
	return strings.Join(parts, "; ")
}

func countRecordsForAccount(records []SubmitStateRecordView, accountID string) int {
	count := 0
	for _, record := range records {
		if record.AccountID == accountID {
			count++
		}
	}
	return count
}

func sortSubmitStateCompareEntries(entries []SubmitStateCompareEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Outcome != entries[j].Outcome {
			return entries[i].Outcome < entries[j].Outcome
		}
		if entries[i].SubmitIdentity != entries[j].SubmitIdentity {
			return entries[i].SubmitIdentity < entries[j].SubmitIdentity
		}
		return entries[i].BrokerOrderID < entries[j].BrokerOrderID
	})
}
