package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// DuplicateSubmitDenyReason is a structured fail-closed deny reason for the
// minimal duplicate-submit protection layer.
type DuplicateSubmitDenyReason string

const (
	DuplicateSubmitUnknownState    DuplicateSubmitDenyReason = "duplicate_submit_unknown_state"
	DuplicateSubmitInFlight        DuplicateSubmitDenyReason = "duplicate_submit_in_flight"
	DuplicateSubmitAlreadySent     DuplicateSubmitDenyReason = "duplicate_submit_already_submitted"
	DuplicateSubmitStateMismatch   DuplicateSubmitDenyReason = "duplicate_submit_state_mismatch"
	DuplicateSubmitRestartInFlight DuplicateSubmitDenyReason = "duplicate_submit_restart_in_flight"
	DuplicateSubmitRestartUnknown  DuplicateSubmitDenyReason = "duplicate_submit_restart_unknown"
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
	Identity SubmitIdentity      `json:"identity"`
	State    SubmitIdentityState `json:"state"`
}

type persistedSubmitIdentities struct {
	Version int                    `json:"version"`
	Records []submitIdentityRecord `json:"records"`
}

type submitIdentityRegistry struct {
	mu        sync.Mutex
	records   map[string]submitIdentityRecord
	loaded    bool
	uncertain bool
}

var (
	liveSubmitIdentities = &submitIdentityRegistry{records: map[string]submitIdentityRecord{}}

	submitIdentityStatePath = func() (string, error) {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("submit identity state path: %w", err)
		}
		return filepath.Join(dir, "tastytrade-cli", "live_submit_identities.json"), nil
	}
	submitIdentityReadFile  = os.ReadFile
	submitIdentityWriteFile = os.WriteFile
)

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

func validSubmitIdentityState(state SubmitIdentityState) bool {
	return state == SubmitIdentityInFlight || state == SubmitIdentitySubmitted
}

func (r *submitIdentityRegistry) ensureLoaded() error {
	if r == nil {
		return errors.New("nil registry")
	}
	if r.loaded {
		return nil
	}
	if r.records == nil {
		r.records = map[string]submitIdentityRecord{}
	}

	path, err := submitIdentityStatePath()
	if err != nil {
		r.uncertain = true
		r.loaded = true
		return err
	}
	data, err := submitIdentityReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			r.loaded = true
			return nil
		}
		r.uncertain = true
		r.loaded = true
		return err
	}

	var persisted persistedSubmitIdentities
	if err := json.Unmarshal(data, &persisted); err != nil {
		r.uncertain = true
		r.loaded = true
		return err
	}
	if persisted.Version != 1 {
		r.uncertain = true
		r.loaded = true
		return fmt.Errorf("unsupported persisted identity state version: %d", persisted.Version)
	}
	for _, record := range persisted.Records {
		if record.Identity.Key == "" || record.Identity.AccountID == "" || record.Identity.IntentID == "" || record.Identity.OrderHash == "" || !validSubmitIdentityState(record.State) {
			r.uncertain = true
			r.loaded = true
			return fmt.Errorf("invalid persisted submit identity state")
		}
		r.records[record.Identity.Key] = record
	}
	r.loaded = true
	return nil
}

func (r *submitIdentityRegistry) saveLocked() error {
	if r == nil || r.records == nil {
		return errors.New("nil submit identity registry")
	}
	path, err := submitIdentityStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	persisted := persistedSubmitIdentities{Version: 1, Records: make([]submitIdentityRecord, 0, len(r.records))}
	for _, record := range r.records {
		persisted.Records = append(persisted.Records, record)
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return submitIdentityWriteFile(path, data, 0o600)
}

func (r *submitIdentityRegistry) reserve(identity SubmitIdentity) DuplicateSubmitCheckResult {
	if r == nil || identity.Key == "" || identity.AccountID == "" || identity.IntentID == "" || identity.OrderHash == "" {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureLoaded(); err != nil || r.uncertain {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitRestartUnknown}
	}

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

	for _, existing := range r.records {
		if existing.Identity.AccountID == identity.AccountID && existing.Identity.OrderHash == identity.OrderHash && existing.State == SubmitIdentityInFlight {
			return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitRestartInFlight, State: existing.State}
		}
	}

	r.records[identity.Key] = submitIdentityRecord{Identity: identity, State: SubmitIdentityInFlight}
	if err := r.saveLocked(); err != nil {
		delete(r.records, identity.Key)
		r.uncertain = true
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitRestartUnknown}
	}
	return DuplicateSubmitCheckResult{Allowed: true, State: SubmitIdentityInFlight}
}

func (r *submitIdentityRegistry) markSubmitted(identity SubmitIdentity) DuplicateSubmitCheckResult {
	if r == nil || identity.Key == "" {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureLoaded(); err != nil || r.uncertain {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitRestartUnknown}
	}

	existing, ok := r.records[identity.Key]
	if !ok {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitUnknownState}
	}
	if existing.Identity != identity {
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitStateMismatch, State: existing.State}
	}
	existing.State = SubmitIdentitySubmitted
	r.records[identity.Key] = existing
	if err := r.saveLocked(); err != nil {
		r.uncertain = true
		return DuplicateSubmitCheckResult{Allowed: false, DenyReason: DuplicateSubmitRestartUnknown, State: existing.State}
	}
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
