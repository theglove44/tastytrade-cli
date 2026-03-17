package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/config"
	exchangeapi "github.com/theglove44/tastytrade-cli/internal/exchange"
	ttexchange "github.com/theglove44/tastytrade-cli/internal/exchange/tastytrade"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

// PreSubmitDenyReason is a structured fail-closed reason for live-submit policy denial.
type PreSubmitDenyReason string

const (
	DenyUnknownState            PreSubmitDenyReason = "unknown_state"
	DenyLiveTradingDisabled     PreSubmitDenyReason = "live_trading_disabled"
	DenyInvalidLiveContext      PreSubmitDenyReason = "invalid_live_context"
	DenyMissingAccountID        PreSubmitDenyReason = "missing_account_id"
	DenyAccountMismatch         PreSubmitDenyReason = "account_mismatch"
	DenyTransportNotApproved    PreSubmitDenyReason = "transport_not_approved"
	DenySafetyCheckFailed       PreSubmitDenyReason = "safety_check_failed"
	DenyDecisionGateUnavailable PreSubmitDenyReason = "decision_gate_unavailable"
	DenyDecisionGateDenied      PreSubmitDenyReason = "decision_gate_denied"
	DenyApprovalMissing         PreSubmitDenyReason = "approval_missing"
	DenyApprovalExpired         PreSubmitDenyReason = "approval_expired"
	DenyConfirmationMissing     PreSubmitDenyReason = "confirmation_missing"
	DenyConfirmationDeclined    PreSubmitDenyReason = "confirmation_declined"
	DenyConfirmationExpired     PreSubmitDenyReason = "confirmation_expired"
	DenyTimeStateInvalid        PreSubmitDenyReason = "time_state_invalid"
	DenyIntentMismatch          PreSubmitDenyReason = "intent_mismatch"
	DenyPayloadMismatch         PreSubmitDenyReason = "payload_mismatch"
)

var (
	preSubmitPolicyNow = func() time.Time { return time.Now().UTC() }
	maxApprovalAge     = 2 * time.Minute
	maxConfirmationAge = 2 * time.Minute
)

// SubmitApproval captures the approved live-submit state bound to a specific
// account, intent, and canonical payload fingerprint.
type SubmitApproval struct {
	Action     string    `json:"action"`
	AccountID  string    `json:"account_id"`
	IntentID   string    `json:"intent_id"`
	OrderHash  string    `json:"order_hash"`
	ApprovedAt time.Time `json:"approved_at"`
}

// SubmitConfirmation captures the explicit acknowledgement bound to a specific
// live-submit intent and order payload fingerprint.
type SubmitConfirmation struct {
	Action         string    `json:"action"`
	AccountID      string    `json:"account_id"`
	IntentID       string    `json:"intent_id"`
	OrderHash      string    `json:"order_hash"`
	Acknowledged   bool      `json:"acknowledged"`
	NonInteractive bool      `json:"non_interactive"`
	ConfirmedAt    time.Time `json:"confirmed_at"`
}

// PreSubmitPolicyInput is the exact state the final live-submit boundary must validate.
type PreSubmitPolicyInput struct {
	Config            *config.Config
	AccountID         string
	IntentID          string
	Order             models.NewOrder
	OrderHash         string
	ApprovedAt        time.Time
	Now               time.Time
	SafetyErr         error
	DecisionView      decisionGateView
	DecisionErr       error
	Approval          *SubmitApproval
	Confirmation      *SubmitConfirmation
	TransportApproved bool
}

// PreSubmitPolicyResult is the final allow/deny result for live broker transmission.
type PreSubmitPolicyResult struct {
	Allowed     bool                  `json:"allowed"`
	DenyReasons []PreSubmitDenyReason `json:"deny_reasons,omitempty"`
}

// SubmitDenialDiagnostics is the compact operator-facing final-boundary denial summary.
type SubmitDenialDiagnostics struct {
	Outcome               string `json:"outcome"`
	PrimaryReason         string `json:"primary_reason,omitempty"`
	IntentID              string `json:"intent_id,omitempty"`
	PayloadHashMatched    bool   `json:"payload_hash_matched"`
	ApprovalAge           string `json:"approval_age,omitempty"`
	ApprovalFreshness     string `json:"approval_freshness,omitempty"`
	ConfirmationAge       string `json:"confirmation_age,omitempty"`
	ConfirmationFreshness string `json:"confirmation_freshness,omitempty"`
	DuplicateState        string `json:"duplicate_state,omitempty"`
}

var isApprovedLiveSubmitTransport = func(ex exchangeapi.Exchange, cfg *config.Config) bool {
	if ex == nil || cfg == nil || !cfg.IsProd() {
		return false
	}
	_, ok := ex.(*ttexchange.Exchange)
	return ok
}

func canonicalOrderHash(order models.NewOrder) (string, error) {
	data, err := json.Marshal(order)
	if err != nil {
		return "", fmt.Errorf("canonical order hash: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// EvaluatePreSubmitPolicy is the final fail-closed policy boundary immediately
// before live broker transmission.
func EvaluatePreSubmitPolicy(in PreSubmitPolicyInput) PreSubmitPolicyResult {
	result := PreSubmitPolicyResult{Allowed: false}
	deny := func(reason PreSubmitDenyReason) {
		for _, existing := range result.DenyReasons {
			if existing == reason {
				return
			}
		}
		result.DenyReasons = append(result.DenyReasons, reason)
	}

	now := in.Now
	if now.IsZero() {
		deny(DenyTimeStateInvalid)
	}

	if in.Config == nil {
		deny(DenyUnknownState)
		return result
	}
	if !in.Config.LiveTrading {
		deny(DenyLiveTradingDisabled)
	}
	if !in.Config.IsProd() {
		deny(DenyInvalidLiveContext)
	}
	if strings.TrimSpace(in.AccountID) == "" {
		deny(DenyMissingAccountID)
	}
	if strings.TrimSpace(in.IntentID) == "" || strings.TrimSpace(in.OrderHash) == "" {
		deny(DenyUnknownState)
	}
	if !in.TransportApproved {
		deny(DenyTransportNotApproved)
	}
	if in.SafetyErr != nil {
		deny(DenySafetyCheckFailed)
	}
	if !in.DecisionView.Available {
		deny(DenyDecisionGateUnavailable)
	}
	if in.DecisionErr != nil || in.DecisionView.Decision.Outcome == reconciler.GateBlock {
		deny(DenyDecisionGateDenied)
	}

	if in.Approval == nil {
		deny(DenyApprovalMissing)
	} else {
		if strings.TrimSpace(in.Approval.IntentID) == "" || strings.TrimSpace(in.Approval.OrderHash) == "" {
			deny(DenyUnknownState)
		}
		if in.Approval.AccountID != in.AccountID {
			deny(DenyAccountMismatch)
		}
		if in.Approval.IntentID != in.IntentID {
			deny(DenyIntentMismatch)
		}
		if in.Approval.OrderHash != in.OrderHash {
			deny(DenyPayloadMismatch)
		}
		if in.Approval.ApprovedAt.IsZero() || in.ApprovedAt.IsZero() {
			deny(DenyApprovalMissing)
		} else if in.Approval.ApprovedAt != in.ApprovedAt {
			deny(DenyTimeStateInvalid)
		} else if in.ApprovedAt.After(now) {
			deny(DenyTimeStateInvalid)
		} else if now.Sub(in.ApprovedAt) > maxApprovalAge {
			deny(DenyApprovalExpired)
		}
	}

	if in.Confirmation == nil {
		deny(DenyConfirmationMissing)
	} else {
		if !in.Confirmation.Acknowledged {
			deny(DenyConfirmationDeclined)
		}
		if strings.TrimSpace(in.Confirmation.IntentID) == "" || strings.TrimSpace(in.Confirmation.OrderHash) == "" {
			deny(DenyUnknownState)
		}
		if in.Confirmation.AccountID != in.AccountID {
			deny(DenyAccountMismatch)
		}
		if in.Confirmation.IntentID != in.IntentID {
			deny(DenyIntentMismatch)
		}
		if in.Confirmation.OrderHash != in.OrderHash {
			deny(DenyPayloadMismatch)
		}
		if in.Confirmation.ConfirmedAt.IsZero() {
			deny(DenyConfirmationMissing)
		} else if in.Confirmation.ConfirmedAt.After(now) {
			deny(DenyTimeStateInvalid)
		} else if now.Sub(in.Confirmation.ConfirmedAt) > maxConfirmationAge {
			deny(DenyConfirmationExpired)
		} else if !in.ApprovedAt.IsZero() && in.Confirmation.ConfirmedAt.Before(in.ApprovedAt) {
			deny(DenyTimeStateInvalid)
		}
	}

	currentHash, err := canonicalOrderHash(in.Order)
	if err != nil {
		deny(DenyUnknownState)
	} else if currentHash != in.OrderHash {
		deny(DenyPayloadMismatch)
	}

	if len(result.DenyReasons) == 0 {
		result.Allowed = true
	}
	return result
}

func freshnessStatus(now, ts time.Time, maxAge time.Duration) (age string, freshness string) {
	if now.IsZero() || ts.IsZero() {
		return "unknown", "missing"
	}
	if ts.After(now) {
		return "unknown", "invalid"
	}
	ageDur := now.Sub(ts)
	age = fmt.Sprintf("%ds", int(ageDur.Seconds()))
	if ageDur > maxAge {
		return age, "expired"
	}
	return age, "fresh"
}

func buildSubmitDenialDiagnostics(in PreSubmitPolicyInput, result PreSubmitPolicyResult, duplicate *DuplicateSubmitCheckResult) SubmitDenialDiagnostics {
	d := SubmitDenialDiagnostics{
		Outcome:        "deny",
		IntentID:       in.IntentID,
		DuplicateState: "not_checked",
	}
	if result.Allowed {
		d.Outcome = "allow"
	}
	if len(result.DenyReasons) > 0 {
		d.PrimaryReason = string(result.DenyReasons[0])
	}
	if currentHash, err := canonicalOrderHash(in.Order); err == nil && in.OrderHash != "" {
		d.PayloadHashMatched = currentHash == in.OrderHash
	}
	now := in.Now
	if now.IsZero() {
		now = preSubmitPolicyNow()
	}
	d.ApprovalAge, d.ApprovalFreshness = freshnessStatus(now, in.ApprovedAt, maxApprovalAge)
	if in.Confirmation != nil {
		d.ConfirmationAge, d.ConfirmationFreshness = freshnessStatus(now, in.Confirmation.ConfirmedAt, maxConfirmationAge)
	} else {
		d.ConfirmationAge, d.ConfirmationFreshness = "unknown", "missing"
	}
	if duplicate != nil {
		if duplicate.State != "" {
			d.DuplicateState = string(duplicate.State)
		} else if duplicate.DenyReason != "" {
			d.DuplicateState = string(duplicate.DenyReason)
		}
	}
	return d
}

func renderSubmitDenialDiagnostics(d SubmitDenialDiagnostics) {
	fmt.Println("LIVE SUBMIT DENIED")
	fmt.Printf("  outcome=%s primary_reason=%s intent_id=%s\n", d.Outcome, d.PrimaryReason, d.IntentID)
	fmt.Printf("  payload_hash_matched=%t\n", d.PayloadHashMatched)
	fmt.Printf("  approval_age=%s approval_freshness=%s\n", d.ApprovalAge, d.ApprovalFreshness)
	fmt.Printf("  confirmation_age=%s confirmation_freshness=%s\n", d.ConfirmationAge, d.ConfirmationFreshness)
	fmt.Printf("  duplicate_state=%s\n", d.DuplicateState)
}

func logPreSubmitPolicyResult(log *zap.Logger, result PreSubmitPolicyResult, accountID, intentID string, decision decisionGateView) {
	if result.Allowed {
		log.Info("pre-submit policy: allow",
			zap.String("account_id", accountID),
			zap.String("intent_id", intentID),
			zap.String("gate_outcome", string(decision.Decision.Outcome)),
			zap.String("reconcile_status", string(decision.Decision.ReconcileStatus)),
			zap.String("reconcile_policy", string(decision.Decision.ReconcilePolicy)),
		)
		return
	}
	reasons := make([]string, 0, len(result.DenyReasons))
	for _, reason := range result.DenyReasons {
		reasons = append(reasons, string(reason))
	}
	log.Warn("pre-submit policy: deny",
		zap.String("account_id", accountID),
		zap.String("intent_id", intentID),
		zap.String("gate_outcome", string(decision.Decision.Outcome)),
		zap.String("reconcile_status", string(decision.Decision.ReconcileStatus)),
		zap.String("reconcile_policy", string(decision.Decision.ReconcilePolicy)),
		zap.Strings("deny_reasons", reasons),
	)
}
