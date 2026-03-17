package reconciler

// GateOutcome is the lightweight operational decision produced from the latest
// reconcile result for confidence-dependent actions.
type GateOutcome string

const (
	GateAllow            GateOutcome = "allowed"
	GateAllowWithWarning GateOutcome = "allowed_with_warning"
	GateBlock            GateOutcome = "blocked"
)

// GateDecision is the lightweight decision-gating view derived from the
// existing reconcile result and outcome policy.
type GateDecision struct {
	Outcome                   GateOutcome  `json:"outcome"`
	ReconcileStatus           Status       `json:"reconcile_status"`
	ReconcilePolicy           HandlingMode `json:"reconcile_policy"`
	Degraded                  bool         `json:"degraded"`
	SuppressConfidenceActions bool         `json:"suppress_confidence_actions"`
	Reason                    string       `json:"reason,omitempty"`
}

// GateDecisionForResult converts a reconcile result into a lightweight gating
// decision for confidence-dependent actions. This is intentionally a thin
// mapping layer on top of PolicyForResult, not a separate rules engine.
func GateDecisionForResult(result Result) GateDecision {
	policy := PolicyForResult(result)
	decision := GateDecision{
		Outcome:                   GateAllow,
		ReconcileStatus:           result.Status,
		ReconcilePolicy:           policy.Handling,
		Degraded:                  policy.Degraded,
		SuppressConfidenceActions: policy.SuppressConfidenceActions,
	}

	switch result.Status {
	case StatusOK:
		return decision
	case StatusDriftDetected:
		decision.Outcome = GateAllowWithWarning
		decision.Reason = "reconciler detected runtime drift"
		return decision
	case StatusPartial:
		decision.Outcome = GateAllowWithWarning
		decision.Reason = "reconciler completed partially; runtime confidence is degraded"
		return decision
	case StatusError:
		decision.Outcome = GateBlock
		decision.Reason = "reconciler policy suppresses confidence-dependent actions"
		return decision
	default:
		if policy.SuppressConfidenceActions {
			decision.Outcome = GateBlock
			decision.Reason = "reconciler policy suppresses confidence-dependent actions"
			return decision
		}
		if policy.Degraded {
			decision.Outcome = GateAllowWithWarning
			decision.Reason = "reconciler policy marks runtime degraded"
		}
		return decision
	}
}
