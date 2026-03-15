package cmd

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

// decisionGateView is the operator-facing gating view used by confidence-
// dependent CLI action paths.
type decisionGateView struct {
	Available bool
	Action    string
	Decision  reconciler.GateDecision
}

func currentDecisionGate(action string, rec reconciler.Reconciler) decisionGateView {
	view := decisionGateView{
		Action: action,
		Decision: reconciler.GateDecision{
			Outcome:         reconciler.GateAllow,
			ReconcileStatus: reconciler.Status("not_yet_available"),
			ReconcilePolicy: reconciler.HandlingMode("not_yet_available"),
		},
	}
	if rec == nil {
		return view
	}
	latest, ok := rec.LatestResult()
	if !ok {
		return view
	}
	view.Available = true
	view.Decision = reconciler.GateDecisionForResult(latest)
	return view
}

func enforceDecisionGate(action string, rec reconciler.Reconciler, log *zap.Logger) error {
	view := currentDecisionGate(action, rec)
	fields := []zap.Field{
		zap.String("action", action),
		zap.String("gate_outcome", string(view.Decision.Outcome)),
		zap.String("reconcile_status", string(view.Decision.ReconcileStatus)),
		zap.String("reconcile_policy", string(view.Decision.ReconcilePolicy)),
		zap.Bool("reconcile_degraded", view.Decision.Degraded),
		zap.Bool("suppress_confidence_actions", view.Decision.SuppressConfidenceActions),
	}
	if view.Decision.Reason != "" {
		fields = append(fields, zap.String("reason", view.Decision.Reason))
	}

	switch view.Decision.Outcome {
	case reconciler.GateAllowWithWarning:
		log.Warn("decision gate: proceeding with degraded confidence", fields...)
		return nil
	case reconciler.GateBlock:
		log.Warn("decision gate: blocked confidence-dependent action", fields...)
		return fmt.Errorf("%s blocked by reconcile policy: status=%s policy=%s reason=%s",
			action,
			view.Decision.ReconcileStatus,
			view.Decision.ReconcilePolicy,
			view.Decision.Reason,
		)
	default:
		if view.Available {
			log.Info("decision gate: confidence check passed", fields...)
		}
		return nil
	}
}
