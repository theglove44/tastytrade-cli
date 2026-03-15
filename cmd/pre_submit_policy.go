package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

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
	DenyConfirmationMissing     PreSubmitDenyReason = "confirmation_missing"
	DenyConfirmationDeclined    PreSubmitDenyReason = "confirmation_declined"
	DenyIntentMismatch          PreSubmitDenyReason = "intent_mismatch"
	DenyPayloadMismatch         PreSubmitDenyReason = "payload_mismatch"
)

// SubmitConfirmation captures the explicit acknowledgement bound to a specific
// live-submit intent and order payload fingerprint.
type SubmitConfirmation struct {
	Action         string `json:"action"`
	AccountID      string `json:"account_id"`
	IntentID       string `json:"intent_id"`
	OrderHash      string `json:"order_hash"`
	Acknowledged   bool   `json:"acknowledged"`
	NonInteractive bool   `json:"non_interactive"`
}

// PreSubmitPolicyInput is the exact state the final live-submit boundary must validate.
type PreSubmitPolicyInput struct {
	Config            *config.Config
	AccountID         string
	IntentID          string
	Order             models.NewOrder
	OrderHash         string
	SafetyErr         error
	DecisionView      decisionGateView
	DecisionErr       error
	Confirmation      *SubmitConfirmation
	TransportApproved bool
}

// PreSubmitPolicyResult is the final allow/deny result for live broker transmission.
type PreSubmitPolicyResult struct {
	Allowed     bool                  `json:"allowed"`
	DenyReasons []PreSubmitDenyReason `json:"deny_reasons,omitempty"`
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
