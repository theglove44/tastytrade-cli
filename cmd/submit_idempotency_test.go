package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func freshSubmitIdentityRegistry() *submitIdentityRegistry {
	return &submitIdentityRegistry{records: map[string]submitIdentityRecord{}}
}

func withSubmitIdentityPersistence(t *testing.T) *submitIdentityRegistry {
	t.Helper()
	dir := t.TempDir()
	origPath := submitIdentityStatePath
	origRead := submitIdentityReadFile
	origWrite := submitIdentityWriteFile
	t.Cleanup(func() {
		submitIdentityStatePath = origPath
		submitIdentityReadFile = origRead
		submitIdentityWriteFile = origWrite
	})
	submitIdentityStatePath = func() (string, error) {
		return filepath.Join(dir, "live_submit_identities.json"), nil
	}
	submitIdentityReadFile = os.ReadFile
	submitIdentityWriteFile = os.WriteFile
	return freshSubmitIdentityRegistry()
}

func TestDeriveSubmitIdentity(t *testing.T) {
	identity, err := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if err != nil {
		t.Fatalf("deriveSubmitIdentity: %v", err)
	}
	if identity.Key == "" {
		t.Fatal("identity.Key = empty, want stable derived key")
	}
	if identity.AccountID != "ACCT-1" || identity.IntentID != "intent-1" || identity.OrderHash != "hash-1" {
		t.Fatalf("identity = %+v, want bound account/intent/hash", identity)
	}
}

func TestSubmitIdentity_FirstSubmitAllowed(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	result := r.reserve(identity)
	if !result.Allowed || result.State != SubmitIdentityInFlight {
		t.Fatalf("result = %+v, want first submit allowed in-flight", result)
	}
}

func TestSubmitIdentity_ExactDuplicateDenied(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if first := r.reserve(identity); !first.Allowed {
		t.Fatalf("first reserve = %+v, want allowed", first)
	}
	second := r.reserve(identity)
	if second.Allowed || second.DenyReason != DuplicateSubmitInFlight {
		t.Fatalf("second reserve = %+v, want duplicate in-flight deny", second)
	}
}

func TestSubmitIdentity_SubmittedDuplicateDenied(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	if first := r.reserve(identity); !first.Allowed {
		t.Fatalf("first reserve = %+v, want allowed", first)
	}
	if marked := r.markSubmitted(identity); !marked.Allowed || marked.State != SubmitIdentitySubmitted {
		t.Fatalf("markSubmitted = %+v, want submitted", marked)
	}
	second := r.reserve(identity)
	if second.Allowed || second.DenyReason != DuplicateSubmitAlreadySent {
		t.Fatalf("second reserve = %+v, want duplicate submitted deny", second)
	}
}

func TestSubmitIdentity_UnknownStateDenied(t *testing.T) {
	var nilRegistry *submitIdentityRegistry
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	result := nilRegistry.reserve(identity)
	if result.Allowed || result.DenyReason != DuplicateSubmitUnknownState {
		t.Fatalf("result = %+v, want unknown-state deny", result)
	}
}

func TestSubmitIdentity_MismatchedPriorStateDenied(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	r.records[identity.Key] = submitIdentityRecord{
		Identity: SubmitIdentity{Key: identity.Key, AccountID: "OTHER", IntentID: identity.IntentID, OrderHash: identity.OrderHash},
		State:    SubmitIdentityInFlight,
	}
	result := r.reserve(identity)
	if result.Allowed || result.DenyReason != DuplicateSubmitStateMismatch {
		t.Fatalf("result = %+v, want state-mismatch deny", result)
	}
}

func TestSubmitIdentity_RestartWithPriorInFlightDenied(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	prior, _ := deriveSubmitIdentity("ACCT-1", "intent-old", "hash-1")
	if first := r.reserve(prior); !first.Allowed {
		t.Fatalf("first reserve = %+v, want allowed", first)
	}

	r2 := freshSubmitIdentityRegistry()
	retry, _ := deriveSubmitIdentity("ACCT-1", "intent-new", "hash-1")
	result := r2.reserve(retry)
	if result.Allowed || result.DenyReason != DuplicateSubmitRestartInFlight {
		t.Fatalf("result = %+v, want restart in-flight deny", result)
	}
}

func TestSubmitIdentity_RestartWithUnknownPersistedStateDenied(t *testing.T) {
	r := withSubmitIdentityPersistence(t)
	path, _ := submitIdentityStatePath()
	bad := persistedSubmitIdentities{Version: 1, Records: []submitIdentityRecord{{State: SubmitIdentityState("mystery")}}}
	data, _ := json.Marshal(bad)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r2 := freshSubmitIdentityRegistry()
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	result := r2.reserve(identity)
	if result.Allowed || result.DenyReason != DuplicateSubmitRestartUnknown {
		t.Fatalf("result = %+v, want restart unknown deny", result)
	}
	_ = r // avoid unused in some toolchains
}

func TestSubmitIdentity_CleanRestartAllowsNormalSubmitFlow(t *testing.T) {
	_ = withSubmitIdentityPersistence(t)
	r := freshSubmitIdentityRegistry()
	identity, _ := deriveSubmitIdentity("ACCT-1", "intent-1", "hash-1")
	result := r.reserve(identity)
	if !result.Allowed {
		t.Fatalf("result = %+v, want clean-state allow", result)
	}
}
