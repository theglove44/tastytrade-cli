package cmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func setupSubmitStateTest(t *testing.T) *submitIdentityRegistry {
	t.Helper()
	r := withSubmitIdentityPersistence(t)
	origFlagJSON := flagJSON
	origFlagSubmitStateIdentity := flagSubmitStateIdentity
	origFlagSubmitStateYes := flagSubmitStateYes
	origFlagSubmitStateCompareLimit := flagSubmitStateCompareLimit
	origFlagSubmitStateCompareAccount := flagSubmitStateCompareAccount
	origFlagSubmitStateCompareOutcome := flagSubmitStateCompareOutcome
	origConfirm := submitStateConfirmIn
	origNow := preSubmitPolicyNow
	t.Cleanup(func() {
		flagJSON = origFlagJSON
		flagSubmitStateIdentity = origFlagSubmitStateIdentity
		flagSubmitStateYes = origFlagSubmitStateYes
		flagSubmitStateCompareLimit = origFlagSubmitStateCompareLimit
		flagSubmitStateCompareAccount = origFlagSubmitStateCompareAccount
		flagSubmitStateCompareOutcome = origFlagSubmitStateCompareOutcome
		submitStateConfirmIn = origConfirm
		preSubmitPolicyNow = origNow
	})
	flagJSON = false
	flagSubmitStateIdentity = ""
	flagSubmitStateYes = false
	flagSubmitStateCompareLimit = 25
	flagSubmitStateCompareAccount = ""
	flagSubmitStateCompareOutcome = ""
	submitStateConfirmIn = strings.NewReader("")
	preSubmitPolicyNow = func() time.Time { return time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC) }
	liveSubmitIdentities = r
	return r
}

func captureStdoutOnly(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(data)
}

func TestSubmitStateInspect_CleanState(t *testing.T) {
	setupSubmitStateTest(t)
	stdout := captureStdoutOnly(t, func() {
		if err := runSubmitStateInspect(context.Background()); err != nil {
			t.Fatalf("runSubmitStateInspect: %v", err)
		}
	})
	if !strings.Contains(stdout, "No persisted live submit state records.") {
		t.Fatalf("stdout = %q, want clean-state message", stdout)
	}
}

func TestSubmitStateInspect_PersistedInFlightState(t *testing.T) {
	r := setupSubmitStateTest(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}
	stdout := captureStdoutOnly(t, func() {
		if err := runSubmitStateInspect(context.Background()); err != nil {
			t.Fatalf("runSubmitStateInspect: %v", err)
		}
	})
	for _, want := range []string{"PERSISTED LIVE SUBMIT STATE", "state=in_flight", "account=ACCT-1", "intent=intent-1", "Reset only clears local safety state"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestSubmitStateClear_DeniedWithoutExplicitConfirmation(t *testing.T) {
	r := setupSubmitStateTest(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}
	flagSubmitStateIdentity = identity.Key
	submitStateConfirmIn = strings.NewReader("no\n")
	stdout := captureStdoutOnly(t, func() {
		err := runSubmitStateClear(context.Background())
		if err == nil {
			t.Fatal("runSubmitStateClear error = nil, want operator decline")
		}
	})
	if !strings.Contains(stdout, "LOCAL LIVE SUBMIT STATE CLEAR") || !strings.Contains(stdout, "submit-state clear declined by operator") {
		t.Fatalf("stdout = %q, want confirmation refusal output", stdout)
	}
}

func TestSubmitStateClear_ConfirmedResetClearsLocalPersistedState(t *testing.T) {
	r := setupSubmitStateTest(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if result := r.reserve(identity); !result.Allowed {
		t.Fatalf("reserve = %+v, want allowed", result)
	}
	flagSubmitStateIdentity = identity.Key
	submitStateConfirmIn = strings.NewReader("clear\n")
	stdout := captureStdoutOnly(t, func() {
		if err := runSubmitStateClear(context.Background()); err != nil {
			t.Fatalf("runSubmitStateClear: %v", err)
		}
	})
	if !strings.Contains(stdout, "✓ LOCAL LIVE SUBMIT STATE CLEARED") {
		t.Fatalf("stdout = %q, want cleared output", stdout)
	}
	views, _, err := r.inspect()
	if err != nil {
		t.Fatalf("inspect after clear: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("views = %+v, want empty after clear", views)
	}
}

func TestSubmitStateInspect_InvalidPersistedStateHandledSafely(t *testing.T) {
	setupSubmitStateTest(t)
	path, _ := submitIdentityStatePath()
	if err := os.WriteFile(path, []byte(`{"version":1,"records":[{"state":"broken"}]}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	liveSubmitIdentities = freshSubmitIdentityRegistry()
	stdout := captureStdoutOnly(t, func() {
		err := runSubmitStateInspect(context.Background())
		if err == nil {
			t.Fatal("runSubmitStateInspect error = nil, want safe deny")
		}
	})
	if !strings.Contains(stdout, "LIVE SUBMIT STATE INSPECTION DENIED") || !strings.Contains(stdout, string(DuplicateSubmitRestartUnknown)) {
		t.Fatalf("stdout = %q, want invalid-state deny output", stdout)
	}
}
