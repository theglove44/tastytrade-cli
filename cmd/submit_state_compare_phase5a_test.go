package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/theglove44/tastytrade-cli/config"
)

func TestRunSubmitStateCompare_HumanReadableShowsNextActions(t *testing.T) {
	r := setupSubmitStateTest(t)
	oldCfg, oldEx, oldFlagJSON, oldLimit := cfg, ex, flagJSON, flagSubmitStateCompareLimit
	defer func() { cfg, ex, flagJSON, flagSubmitStateCompareLimit = oldCfg, oldEx, oldFlagJSON, oldLimit }()

	identity, err := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if err != nil {
		t.Fatalf("deriveSubmitIdentity: %v", err)
	}
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}

	cfg = &config.Config{AccountID: "ACCT-1"}
	ex = &submitStateCompareTestExchange{}
	flagJSON = false
	flagSubmitStateCompareLimit = 5

	stdout := captureStdout(t, func() {
		if err := runSubmitStateCompare(context.Background()); err != nil {
			t.Fatalf("runSubmitStateCompare: %v", err)
		}
	})
	for _, want := range []string{"outcome=local_uncertain_no_broker_match", "next_action=re-check broker visibility using broker-orders live and broker-orders recent", "next_action=do not retry or clear local state automatically"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}
