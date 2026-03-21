package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagSubmitStateIdentity        string
	flagSubmitStateInspectIdentity string
	flagSubmitStateYes             bool
	submitStateConfirmIn           io.Reader = os.Stdin
)

type SubmitStateRecordView struct {
	SubmitIdentity string `json:"submit_identity"`
	AccountID      string `json:"account_id"`
	IntentID       string `json:"intent_id"`
	OrderHash      string `json:"order_hash"`
	State          string `json:"state"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	DenyReason     string `json:"deny_reason,omitempty"`
}

type SubmitStateInspectOutput struct {
	Status     string                  `json:"status"`
	DenyReason string                  `json:"deny_reason,omitempty"`
	Records    []SubmitStateRecordView `json:"records,omitempty"`
}

var submitStateCmd = &cobra.Command{
	Use:   "submit-state",
	Short: "Inspect, compare, or clear persisted live submit safety state",
	Long: `Operator-only commands for inspecting, comparing, or clearing persisted live submit safety state.

Use these commands with broker order inspection for manual reconciliation workflows.
They only affect local duplicate-submit / restart-recovery safety state.
They do NOT confirm broker outcome or reconcile broker-side orders.`,
}

var submitStateInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect persisted live submit safety state, optionally by identity",
	Long: `Inspect persisted live submit safety state.

Use --identity to focus on one local submit state target before a manual clear.
This command is read-only and does not change local or broker state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmitStateInspect(cmd.Context())
	},
}

var submitStateClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear one persisted live submit state record after broker verification",
	Long: `Clear one persisted live submit state record after you have manually verified broker truth.

This command only removes local duplicate-submit / restart-recovery safety state.
Use it as explicit post-verification cleanup, not as a reconciliation step.
It does not confirm broker outcome or reconcile broker-side orders.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmitStateClear(cmd.Context())
	},
}

func init() {
	submitStateInspectCmd.Flags().StringVar(&flagSubmitStateInspectIdentity, "identity", "", "Optional persisted submit identity key to inspect")
	submitStateClearCmd.Flags().StringVar(&flagSubmitStateIdentity, "identity", "", "Persisted submit identity key to clear")
	submitStateClearCmd.Flags().BoolVar(&flagSubmitStateYes, "yes", false, "Acknowledge local state clear non-interactively")
	_ = submitStateClearCmd.MarkFlagRequired("identity")
	submitStateCmd.AddCommand(submitStateInspectCmd, submitStateClearCmd)
}

func runSubmitStateInspect(_ context.Context) error {
	records, denyReason, err := liveSubmitIdentities.inspect()
	if flagJSON {
		out := SubmitStateInspectOutput{Status: "ok", Records: records}
		if err != nil {
			out.Status = "deny"
			out.DenyReason = string(denyReason)
			out.Records = nil
		}
		return printJSON(out)
	}
	if err != nil {
		fmt.Println("LIVE SUBMIT STATE INSPECTION DENIED")
		fmt.Printf("  status=deny primary_reason=%s\n", denyReason)
		fmt.Println("  persisted submit state is invalid or ambiguous; local safety state must be treated as uncertain")
		return fmt.Errorf("submit-state inspect denied: %s", denyReason)
	}
	if flagSubmitStateInspectIdentity != "" {
		fmt.Printf("Inspecting persisted live submit state for submit_identity=%s\n", flagSubmitStateInspectIdentity)
	}
	if len(records) == 0 {
		if flagSubmitStateInspectIdentity != "" {
			fmt.Printf("No persisted live submit state record found for submit_identity=%s.\n", flagSubmitStateInspectIdentity)
		} else {
			fmt.Println("No persisted live submit state records.")
		}
		return nil
	}
	records = filterSubmitStateRecordsByIdentity(records, flagSubmitStateInspectIdentity)
	if len(records) == 0 {
		if flagSubmitStateInspectIdentity != "" {
			fmt.Printf("No persisted live submit state record found for submit_identity=%s.\n", flagSubmitStateInspectIdentity)
			return nil
		}
		fmt.Println("No persisted live submit state records.")
		return nil
	}
	fmt.Println("PERSISTED LIVE SUBMIT STATE")
	for _, record := range records {
		fmt.Printf("- submit_identity=%s state=%s account=%s intent=%s\n", record.SubmitIdentity, record.State, record.AccountID, record.IntentID)
		fmt.Printf("  order_hash=%s\n", record.OrderHash)
		if record.CreatedAt != "" || record.UpdatedAt != "" {
			fmt.Printf("  created_at=%s updated_at=%s\n", record.CreatedAt, record.UpdatedAt)
		}
		if record.DenyReason != "" {
			fmt.Printf("  deny_reason=%s\n", record.DenyReason)
		}
	}
	fmt.Println("After manual broker verification, clear local state explicitly with tt submit-state clear --identity <submit-identity>.")
	fmt.Println("Reset only clears local safety state; it does not confirm broker outcome.")
	return nil
}

func runSubmitStateClear(_ context.Context) error {
	if !flagSubmitStateYes {
		fmt.Println("LOCAL LIVE SUBMIT STATE CLEAR")
		fmt.Println("This is explicit post-verification local cleanup only.")
		fmt.Println("First inspect the target with tt submit-state inspect --identity <submit-identity>.")
		fmt.Println("Before clearing, confirm broker truth manually with tt broker-orders live and tt broker-orders recent --limit N.")
		fmt.Println("It does NOT confirm broker outcome or reconcile broker-side orders.")
		fmt.Printf("Target identity: %s\n", flagSubmitStateIdentity)
		fmt.Print("Type 'clear' to confirm local state reset: ")
		reader := bufio.NewReader(submitStateConfirmIn)
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("submit-state clear confirmation failed: %w", err)
		}
		if strings.TrimSpace(strings.ToLower(line)) != "clear" {
			fmt.Println("submit-state clear declined by operator")
			return fmt.Errorf("submit-state clear aborted: operator declined confirmation")
		}
	}
	if err := liveSubmitIdentities.clear(flagSubmitStateIdentity); err != nil {
		return printSubmitStateClearOutcome(flagSubmitStateIdentity, err)
	}
	if !flagJSON {
		fmt.Println("✓ LOCAL LIVE SUBMIT STATE CLEARED")
		fmt.Printf("  Local duplicate-submit / restart-recovery state removed for submit_identity=%s.\n", flagSubmitStateIdentity)
		fmt.Println("  Broker truth must already have been verified manually; this command does not confirm it.")
	}
	return nil
}

func printSubmitStateClearOutcome(identityKey string, err error) error {
	if flagJSON {
		return err
	}
	errText := err.Error()
	switch {
	case strings.Contains(errText, string(DuplicateSubmitUnknownState)):
		fmt.Printf("No persisted live submit state record found for submit_identity=%s; nothing was cleared.\n", identityKey)
	case strings.Contains(errText, string(DuplicateSubmitRestartUnknown)):
		fmt.Printf("Persisted submit state is uncertain; submit_identity=%s was not cleared.\n", identityKey)
	}
	return err
}

func recordView(record submitIdentityRecord) SubmitStateRecordView {
	view := SubmitStateRecordView{
		SubmitIdentity: record.Identity.Key,
		AccountID:      record.Identity.AccountID,
		IntentID:       record.Identity.IntentID,
		OrderHash:      record.Identity.OrderHash,
		State:          string(record.State),
		DenyReason:     string(record.DenyReason),
	}
	if !record.CreatedAt.IsZero() {
		view.CreatedAt = record.CreatedAt.Format(time.RFC3339)
	}
	if !record.UpdatedAt.IsZero() {
		view.UpdatedAt = record.UpdatedAt.Format(time.RFC3339)
	}
	return view
}

func filterSubmitStateRecordsByIdentity(records []SubmitStateRecordView, identity string) []SubmitStateRecordView {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return records
	}
	filtered := make([]SubmitStateRecordView, 0, 1)
	for _, record := range records {
		if record.SubmitIdentity == identity {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func sortRecordViews(records []SubmitStateRecordView) {
	sort.Slice(records, func(i, j int) bool { return records[i].SubmitIdentity < records[j].SubmitIdentity })
}
