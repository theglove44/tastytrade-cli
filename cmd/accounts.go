package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "List accounts for the authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAccounts(cmd.Context())
	},
}

// AccountsOutput is the stable JSON shape emitted by --json.
// Field names and types must not change without a version bump — this
// is consumed by the automation pipeline.
type AccountsOutput struct {
	Accounts []AccountSummary `json:"accounts"`
}

type AccountSummary struct {
	AccountNumber string `json:"account_number"`
	AccountType   string `json:"account_type"`
	Nickname      string `json:"nickname"`
	IsClosed      bool   `json:"is_closed"`
}

func runAccounts(ctx context.Context) error {
	accounts, err := ex.Accounts(ctx)
	if err != nil {
		return fmt.Errorf("accounts: %w", err)
	}

	if flagJSON {
		out := AccountsOutput{}
		for _, a := range accounts {
			out.Accounts = append(out.Accounts, AccountSummary{
				AccountNumber: a.AccountNumber,
				AccountType:   a.AccountType,
				Nickname:      a.Nickname,
				IsClosed:      a.IsClosed,
			})
		}
		return printJSON(out)
	}

	// Human-readable
	fmt.Printf("%-16s %-20s %-24s %s\n", "ACCOUNT", "TYPE", "NICKNAME", "CLOSED")
	for _, a := range accounts {
		closed := ""
		if a.IsClosed {
			closed = "yes"
		}
		fmt.Printf("%-16s %-20s %-24s %s\n",
			a.AccountNumber, a.AccountType, a.Nickname, closed)
	}
	return nil
}

// printJSON marshals v to indented JSON and writes to stdout.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
