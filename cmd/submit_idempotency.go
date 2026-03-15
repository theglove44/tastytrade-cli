package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// DuplicateSubmitDenyReason is a structured fail-closed deny reason for the
// minimal duplicate-submit protection layer.
type DuplicateSubmitDenyReason string

const (
	DuplicateSubmitUnknownState  DuplicateSubmitDenyReason = "duplicate_submit_unknown_state"
	DuplicateSubmitInFlight      DuplicateSubmitDenyReason = "duplicate_submit_in_flight"
	DuplicateSubmitAlreadySent   DuplicateSubmitDenyReason = "duplicate_submit_already_submitted"
	DuplicateSubmitStateMismatch DuplicateSubmitDenyReason = "duplicate_submit_state_mismatch"
)

// SubmitIdentity is the stable live-submit identity derived from approved
// account context, intent ID, and canonical order payload hash.
type SubmitIdentity struct {
	Key       string `json:"key"`
	AccountID string `json:"account_id"`
	IntentID  string `json:"intent_id"`
	OrderHash string `json:"order_hash"`
}

// SubmitIdentityState is the minimal in-process duplicate-submit state.
type SubmitIdentityState string

const (
	SubmitIdentityInFlight  SubmitIdentityState = "in_flight"
	SubmitIdentitySubmitted SubmitIdentityState = "submitted"
)

// DuplicateSubmitCheckResult is the result of the duplicate-submit boundary check.
type DuplicateSubmitCheckResult struct {
	Allowed    bool                      `json:"allowed"`
	DenyReason DuplicateSubmitDenyReason `json:"deny_reason,omitempty"`
	State      SubmitIdentityState       `json:"state,omitempty"`
}

type submitIdentityRecord struct {
	Identity SubmitIdentity
	State    SubmitIdentityState
}

type submitIdentityRegistry struct {
	mu      sync.Mutex
	records map[string]submitIdentityRecord
}

var liveSubmitIdentities = &submitIdentityRegistry{records: map[string]submitIdentityRecord{}}

func deriveSubmitIdentity(accountID, intentID, orderHash string) (SubmitIdentity, error) {
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(intentID) == "" || strings.TrimSpace(orderHash) == "" {
		return SubmitIdentity{}, fmt.Errorf("derive submit identity: missing account, intent, or order hash")
	}
	raw := accountID + "\x00" + intentID + "\x00" + orderHash
	sum := sha256.Sum256([]byte(raw))
	return SubmitIdentity{
		Key:       hex.EncodeToString(sum[:]),
		AccountID: accountID,
		IntentID:  intentID,
		OrderHash: orderHash,
	}, nil
}

func (r *submitIdentityRegistry) reserve(identity SubmitIdentity) DuplicateSubmitCheckResult {
	if r == nil || r.records == nil {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}
	if identity.Key == "" || identity.AccountID == "" || identity.IntentID == "" || identity.OrderHash == "" {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.records[identity.Key]; ok {
		if existing.Identity != identity {
			return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitStateMismatch, State: existing.State}
		}
		switch existing.State {
		case SubmitIdentityInFlight:
			return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitInFlight, State: existing.State}
		case SubmitIdentitySubmitted:
			return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitAlreadySent, State: existing.State}
		default:
			return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState, State: existing.State}
		}
	}

	r.records[identity.Key] = submitIdentityRecord{Identity: identity, State: SubmitIdentityInFlight}
	return DuplicateSubmitCheckResult{Allowed: true, State: SubmitIdentityInFlight}
}

func (r *submitIdentityRegistry) markSubmitted(identity SubmitIdentity) DuplicateSubmitCheckResult {
	if r == nil || r.records == nil {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}
	if identity.Key == "" {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.records[identity.Key]
	if !ok {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}
	if existing.Identity != identity {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitStateMismatch, State: existing.State}
	}
	existing.State = SubmitIdentitySubmitted
	r.records[identity.Key] = existing
	return DuplicateSubmitCheckResult{Allowed: true, State: SubmitIdentitySubmitted}
}

func logDuplicateSubmitCheck(log *zap.Logger, identity SubmitIdentity, result DuplicateSubmitCheckResult) {
	if result.Allowed {
		log.Info("duplicate submit policy: allow",
			zap.String("submit_identity", identity.Key),
			zap.String("account_id", identity.AccountID),
			zap.String("intent_id", identity.IntentID),
			zap.String("state", string(result.State)),
		)
		return
	}
	log.Warn("duplicate submit policy: deny",
		zap.String("submit_identity", identity.Key),
		zap.String("account_id", identity.AccountID),
		zap.String("intent_id", identity.IntentID),
		zap.String("deny_reason", string(result.DenyReason)),
		zap.String("state", string(result.State)),
	)
}
