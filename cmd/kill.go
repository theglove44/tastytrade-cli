package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/internal/client"
)

var killCmd = &cobra.Command{
	Use:   "kill",
	Short: "Arm the kill switch — immediately halts all order submission",
	Long: `Arms the file-based kill switch, blocking all order submission until 'tt resume' is run.

This is equivalent to:
  touch ~/.config/tastytrade-cli/KILL

The running process checks the kill switch before every order submission.
No restart or redeploy is required — the check is live.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := client.ArmKillSwitch(); err != nil {
			return fmt.Errorf("kill: %w", err)
		}
		kf, _ := client.KillFilePath()
		fmt.Printf("✓ Kill switch ARMED\n")
		fmt.Printf("  File: %s\n", kf)
		fmt.Printf("  All order submission is now blocked.\n")
		fmt.Printf("  Run 'tt resume' to disarm.\n")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Disarm the kill switch — resumes normal order submission",
	Long: `Disarms the file-based kill switch, allowing order submission to resume.

This is equivalent to:
  rm ~/.config/tastytrade-cli/KILL

Note: the TASTYTRADE_KILL_SWITCH=true env var is independent and must be
unset separately if it was set.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check env var kill switch too — warn if still set.
		halted, reason := client.KillSwitch()
		if err := client.DisarmKillSwitch(); err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		kf, _ := client.KillFilePath()
		fmt.Printf("✓ Kill switch file DISARMED\n")
		fmt.Printf("  File removed: %s\n", kf)
		if halted && reason != "" {
			// Kill switch is still active via env var.
			fmt.Printf("\n⚠  WARNING: Kill switch is still ACTIVE via env var.\n")
			fmt.Printf("   Reason: %s\n", reason)
			fmt.Printf("   Unset TASTYTRADE_KILL_SWITCH to fully resume.\n")
		} else {
			fmt.Printf("  Order submission is now permitted (subject to circuit breaker + safety gates).\n")
		}
		return nil
	},
}
