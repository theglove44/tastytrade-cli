package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

func testLiveOrder(t *testing.T) models.NewOrder {
	t.Helper()
	return models.NewOrder{
		OrderType:   "Limit",
		TimeInForce: "Day",
		Price:       "1.00",
		PriceEffect: "Debit",
		Legs: []models.NewOrderLeg{{
			InstrumentType: "Equity",
			Symbol:         "AAPL",
			Quantity:       1,
			Action:         "Buy to Open",
		}},
	}
}

func testPolicyInput(t *testing.T) PreSubmitPolicyInput {
	t.Helper()
	order := testLiveOrder(t)
	hash, err := canonicalOrderHash(order)
	if err != nil {
		t.Fatalf("canonicalOrderHash: %v", err)
	}
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	approvedAt := now.Add(-30 * time.Second)
	confirmedAt := now.Add(-10 * time.Second)
	return PreSubmitPolicyInput{
		Config: &config.Config{
			BaseURL:            config.ProdBaseURL,
			LiveTrading:        true,
			RateLimits:         config.DefaultRateLimits(),
			AccountID:          "TEST123",
			UserAgent:          "test",
			ClientID:           "cid",
			DXLinkURL:          config.DXLinkBaseURL,
			APIVersion:         "",
			AccountStreamerURL: config.AccountStreamerURL,
		},
		AccountID:  "TEST123",
		IntentID:   "intent-1",
		Order:      order,
		OrderHash:  hash,
		ApprovedAt: approvedAt,
		Now:        now,
		DecisionView: decisionGateView{
			Available: true,
			Action:    "submit",
			Decision: reconciler.GateDecision{
				Outcome:         reconciler.GateAllow,
				ReconcileStatus: reconciler.StatusOK,
				ReconcilePolicy: reconciler.HandlingObserve,
			},
		},
		Approval: &SubmitApproval{
			Action:     "submit",
			AccountID:  "TEST123",
			IntentID:   "intent-1",
			OrderHash:  hash,
			ApprovedAt: approvedAt,
		},
		Confirmation: &SubmitConfirmation{
			Action:       "submit",
			AccountID:    "TEST123",
			IntentID:     "intent-1",
			OrderHash:    hash,
			Acknowledged: true,
			ConfirmedAt:  confirmedAt,
		},
		TransportApproved: true,
	}
}

func hasReason(result PreSubmitPolicyResult, reason PreSubmitDenyReason) bool {
	for _, got := range result.DenyReasons {
		if got == reason {
			return true
		}
	}
	return false
}

func TestEvaluatePreSubmitPolicy_HappyPath(t *testing.T) {
	result := EvaluatePreSubmitPolicy(testPolicyInput(t))
	if !result.Allowed {
		t.Fatalf("Allowed = false, deny reasons = %v", result.DenyReasons)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenLiveTradingDisabled(t *testing.T) {
	in := testPolicyInput(t)
	in.Config.LiveTrading = false
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyLiveTradingDisabled) {
		t.Fatalf("result = %+v, want live-trading denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenTransportNotApproved(t *testing.T) {
	in := testPolicyInput(t)
	in.TransportApproved = false
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyTransportNotApproved) {
		t.Fatalf("result = %+v, want transport denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenSafetyCheckFailed(t *testing.T) {
	in := testPolicyInput(t)
	in.SafetyErr = errors.New("kill switch active")
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenySafetyCheckFailed) {
		t.Fatalf("result = %+v, want safety denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenDecisionUnavailable(t *testing.T) {
	in := testPolicyInput(t)
	in.DecisionView.Available = false
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyDecisionGateUnavailable) {
		t.Fatalf("result = %+v, want unavailable decision denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenDecisionBlocked(t *testing.T) {
	in := testPolicyInput(t)
	in.DecisionView.Decision.Outcome = reconciler.GateBlock
	in.DecisionErr = errors.New("submit blocked by reconcile policy")
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyDecisionGateDenied) {
		t.Fatalf("result = %+v, want decision denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_FreshApprovalAndConfirmationAllowed(t *testing.T) {
	result := EvaluatePreSubmitPolicy(testPolicyInput(t))
	if !result.Allowed {
		t.Fatalf("result = %+v, want allowed", result)
	}
}

func TestEvaluatePreSubmitPolicy_ExpiredApprovalDenied(t *testing.T) {
	in := testPolicyInput(t)
	in.ApprovedAt = in.Now.Add(-maxApprovalAge - time.Second)
	in.Approval.ApprovedAt = in.ApprovedAt
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyApprovalExpired) {
		t.Fatalf("result = %+v, want expired approval denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_ExpiredConfirmationDenied(t *testing.T) {
	in := testPolicyInput(t)
	in.Confirmation.ConfirmedAt = in.Now.Add(-maxConfirmationAge - time.Second)
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyConfirmationExpired) {
		t.Fatalf("result = %+v, want expired confirmation denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_MissingTimestampsDenied(t *testing.T) {
	in := testPolicyInput(t)
	in.ApprovedAt = time.Time{}
	in.Approval.ApprovedAt = time.Time{}
	in.Confirmation.ConfirmedAt = time.Time{}
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyApprovalMissing) || !hasReason(result, DenyConfirmationMissing) {
		t.Fatalf("result = %+v, want missing timestamp denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_InvalidTimeStateDenied(t *testing.T) {
	in := testPolicyInput(t)
	in.Now = time.Time{}
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyTimeStateInvalid) {
		t.Fatalf("result = %+v, want invalid time-state denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenConfirmationMissing(t *testing.T) {
	in := testPolicyInput(t)
	in.Confirmation = nil
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyConfirmationMissing) {
		t.Fatalf("result = %+v, want confirmation denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenIntentMismatches(t *testing.T) {
	in := testPolicyInput(t)
	in.Confirmation.IntentID = "different-intent"
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyIntentMismatch) {
		t.Fatalf("result = %+v, want intent mismatch denial", result)
	}
}

func TestEvaluatePreSubmitPolicy_DeniesWhenPayloadChanged(t *testing.T) {
	in := testPolicyInput(t)
	in.Order.Price = "2.00"
	result := EvaluatePreSubmitPolicy(in)
	if result.Allowed || !hasReason(result, DenyPayloadMismatch) {
		t.Fatalf("result = %+v, want payload mismatch denial", result)
	}
}
