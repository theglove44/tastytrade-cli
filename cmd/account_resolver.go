package cmd

import (
	"context"
	"fmt"
	"strings"
)

// resolveAccountID returns the configured account ID, or auto-selects the only
// available account when exactly one account is returned by the API.
func resolveAccountID(ctx context.Context, op string) (string, error) {
	if cfg.AccountID != "" {
		return cfg.AccountID, nil
	}

	accounts, err := ex.Accounts(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: resolve account: %w", op, err)
	}
	if len(accounts) == 0 {
		return "", fmt.Errorf("%s: no accounts returned; set TASTYTRADE_ACCOUNT_ID", op)
	}
	if len(accounts) == 1 {
		return accounts[0].AccountNumber, nil
	}

	available := make([]string, 0, len(accounts))
	for _, a := range accounts {
		if a.AccountNumber != "" {
			available = append(available, a.AccountNumber)
		}
	}
	if len(available) == 0 {
		return "", fmt.Errorf("%s: multiple accounts returned; set TASTYTRADE_ACCOUNT_ID", op)
	}
	return "", fmt.Errorf("%s: multiple accounts returned; set TASTYTRADE_ACCOUNT_ID to one of: %s", op, strings.Join(available, ", "))
}
