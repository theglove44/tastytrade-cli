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
	flagSubmitStateIdentity string
	flagSubmitStateYes      bool
	submitStateConfirmIn    io.Reader = os.Stdin
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
	Short: "Inspect persisted live submit safety state",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmitStateInspect(cmd.Context())
	},
}

var submitStateClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear one persisted live submit state record after manual verification",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSubmitStateClear(cmd.Context())
	},
}

func init() {
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
	if len(records) == 0 {
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
	fmt.Println("Reset only clears local safety state; it does not confirm broker outcome.")
	return nil
}

func runSubmitStateClear(_ context.Context) error {
	if !flagSubmitStateYes {
		fmt.Println("LOCAL LIVE SUBMIT STATE CLEAR")
		fmt.Println("This only clears local duplicate-submit / restart-recovery safety state.")
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
		return err
	}
	if !flagJSON {
		fmt.Println("✓ LOCAL LIVE SUBMIT STATE CLEARED")
		fmt.Println("  Broker outcome remains unknown until manually verified.")
	}
	return nil
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

func sortRecordViews(records []SubmitStateRecordView) {
	sort.Slice(records, func(i, j int) bool { return records[i].SubmitIdentity < records[j].SubmitIdentity })
}
