package reconciler_test

import (
	"testing"

	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

func TestGateDecisionForResult_OK(t *testing.T) {
	decision := reconciler.GateDecisionForResult(reconciler.Result{Status: reconciler.StatusOK})
	if decision.Outcome != reconciler.GateAllow {
		t.Fatalf("Outcome = %q, want %q", decision.Outcome, reconciler.GateAllow)
	}
	if decision.Degraded || decision.SuppressConfidenceActions {
		t.Fatalf("decision = %+v, want non-degraded allowed path", decision)
	}
}

func TestGateDecisionForResult_DriftDetected(t *testing.T) {
	decision := reconciler.GateDecisionForResult(reconciler.Result{Status: reconciler.StatusDriftDetected})
	if decision.Outcome != reconciler.GateAllowWithWarning {
		t.Fatalf("Outcome = %q, want %q", decision.Outcome, reconciler.GateAllowWithWarning)
	}
	if !decision.Degraded || decision.ReconcilePolicy != reconciler.HandlingObserve {
		t.Fatalf("decision = %+v, want degraded observe warning", decision)
	}
}

func TestGateDecisionForResult_Partial(t *testing.T) {
	decision := reconciler.GateDecisionForResult(reconciler.Result{Status: reconciler.StatusPartial})
	if decision.Outcome != reconciler.GateAllowWithWarning {
		t.Fatalf("Outcome = %q, want %q", decision.Outcome, reconciler.GateAllowWithWarning)
	}
	if !decision.Degraded || decision.SuppressConfidenceActions {
		t.Fatalf("decision = %+v, want degraded warning without suppression", decision)
	}
}

func TestGateDecisionForResult_Error(t *testing.T) {
	decision := reconciler.GateDecisionForResult(reconciler.Result{Status: reconciler.StatusError})
	if decision.Outcome != reconciler.GateBlock {
		t.Fatalf("Outcome = %q, want %q", decision.Outcome, reconciler.GateBlock)
	}
	if !decision.Degraded || !decision.SuppressConfidenceActions || decision.ReconcilePolicy != reconciler.HandlingSuppress {
		t.Fatalf("decision = %+v, want suppressed blocked path", decision)
	}
}
