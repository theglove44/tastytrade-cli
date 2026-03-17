package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

type stubAccountResolverExchange struct {
	accounts []models.Account
	err      error
}

func (s stubAccountResolverExchange) Accounts(context.Context) ([]models.Account, error) {
	return s.accounts, s.err
}
func (s stubAccountResolverExchange) Positions(context.Context, string) ([]models.Position, error) {
	return nil, nil
}
func (s stubAccountResolverExchange) Orders(context.Context, string) ([]models.Order, error) {
	return nil, nil
}
func (s stubAccountResolverExchange) RecentOrders(context.Context, string, int) ([]models.Order, error) {
	return nil, nil
}
func (s stubAccountResolverExchange) DryRun(context.Context, string, models.NewOrder, string) (models.DryRunResult, error) {
	return models.DryRunResult{}, nil
}
func (s stubAccountResolverExchange) Submit(context.Context, string, models.NewOrder, string) (models.SubmitResult, error) {
	return models.SubmitResult{}, nil
}
func (s stubAccountResolverExchange) QuoteToken(context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

func TestResolveAccountID_UsesConfiguredAccount(t *testing.T) {
	oldCfg, oldEx := cfg, ex
	defer func() { cfg, ex = oldCfg, oldEx }()

	cfg = &config.Config{AccountID: "5WW46136"}
	ex = stubAccountResolverExchange{accounts: []models.Account{{AccountNumber: "ignored"}}}

	got, err := resolveAccountID(context.Background(), "positions")
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if got != "5WW46136" {
		t.Fatalf("got %q, want configured account", got)
	}
}

func TestResolveAccountID_AutoSelectsSingleAccount(t *testing.T) {
	oldCfg, oldEx := cfg, ex
	defer func() { cfg, ex = oldCfg, oldEx }()

	cfg = &config.Config{}
	ex = stubAccountResolverExchange{accounts: []models.Account{{AccountNumber: "5WW46136"}}}

	got, err := resolveAccountID(context.Background(), "positions")
	if err != nil {
		t.Fatalf("resolveAccountID: %v", err)
	}
	if got != "5WW46136" {
		t.Fatalf("got %q, want single returned account", got)
	}
}

func TestResolveAccountID_MultipleAccountsRequiresExplicitSelection(t *testing.T) {
	oldCfg, oldEx := cfg, ex
	defer func() { cfg, ex = oldCfg, oldEx }()

	cfg = &config.Config{}
	ex = stubAccountResolverExchange{accounts: []models.Account{{AccountNumber: "A1"}, {AccountNumber: "A2"}}}

	_, err := resolveAccountID(context.Background(), "positions")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "set TASTYTRADE_ACCOUNT_ID") || !strings.Contains(msg, "A1") || !strings.Contains(msg, "A2") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveAccountID_PropagatesAccountsError(t *testing.T) {
	oldCfg, oldEx := cfg, ex
	defer func() { cfg, ex = oldCfg, oldEx }()

	cfg = &config.Config{}
	ex = stubAccountResolverExchange{err: errors.New("boom")}

	_, err := resolveAccountID(context.Background(), "positions")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "positions: resolve account: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}
