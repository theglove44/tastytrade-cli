package cmd

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
)

type stubReconciler struct {
	latest reconciler.Result
	ok     bool
}

func (s *stubReconciler) Start(_ context.Context)                 {}
func (s *stubReconciler) LatestResult() (reconciler.Result, bool) { return s.latest, s.ok }

type stubStreamer struct{ status streamer.StreamerStatus }

func (s *stubStreamer) Start(_ context.Context) error   { return nil }
func (s *stubStreamer) Status() streamer.StreamerStatus { return s.status }
func (s *stubStreamer) Name() string                    { return s.status.Name }

func observedWatchLogger(level zapcore.Level) (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(level)
	return zap.New(core), logs
}

func setHeartbeatGauges(tracked, open float64) {
	client.Metrics.TrackedSymbols.Set(tracked)
	client.Metrics.OpenPositions.Set(open)
}

func TestCurrentWatchHeartbeat_StartupPartialState(t *testing.T) {
	setHeartbeatGauges(0, 0)
	view := currentWatchHeartbeat(&stubStreamer{status: streamer.StreamerStatus{Name: "account"}}, &stubStreamer{status: streamer.StreamerStatus{Name: "market"}}, &stubReconciler{})
	if view.AccountStatus != "starting" || view.MarketStatus != "starting" {
		t.Fatalf("streamer statuses = %q/%q, want starting/starting", view.AccountStatus, view.MarketStatus)
	}
	if view.ReconcileStatus != "not_yet_available" {
		t.Fatalf("reconcile status = %q, want not_yet_available", view.ReconcileStatus)
	}
	if view.ReconcileLastRunAt != "n/a" {
		t.Fatalf("reconcile_last_run_at = %q, want n/a", view.ReconcileLastRunAt)
	}
	if view.ReconcilePolicy != "not_yet_available" {
		t.Fatalf("reconcile policy = %q, want not_yet_available", view.ReconcilePolicy)
	}
	if view.SuppressConfidenceActions {
		t.Fatal("SuppressConfidenceActions = true before any reconcile run")
	}
}

func TestCurrentWatchHeartbeat_Healthy(t *testing.T) {
	setHeartbeatGauges(4, 4)
	view := currentWatchHeartbeat(
		&stubStreamer{status: streamer.StreamerStatus{Name: "account", Connected: true}},
		&stubStreamer{status: streamer.StreamerStatus{Name: "market", Connected: true}},
		&stubReconciler{ok: true, latest: reconciler.Result{RunAt: time.Now().UTC(), Status: reconciler.StatusOK, MismatchCount: 0}},
	)
	if view.Degraded {
		t.Fatalf("Degraded = true, want false: %+v", view)
	}
	if view.AccountStatus != "up" || view.MarketStatus != "up" || view.ReconcileStatus != "ok" {
		t.Fatalf("view = %+v, want healthy heartbeat", view)
	}
	if view.TrackedSymbols != 4 || view.OpenPositions != 4 {
		t.Fatalf("metric counts = %d/%d, want 4/4", view.TrackedSymbols, view.OpenPositions)
	}
	if view.ReconcilePolicy != "observe" {
		t.Fatalf("ReconcilePolicy = %q, want observe", view.ReconcilePolicy)
	}
	if view.SuppressConfidenceActions {
		t.Fatal("SuppressConfidenceActions = true, want false")
	}
}

func TestLogWatchHeartbeat_DriftDegraded(t *testing.T) {
	setHeartbeatGauges(3, 2)
	log, logs := observedWatchLogger(zapcore.InfoLevel)
	logWatchHeartbeat(log,
		&stubStreamer{status: streamer.StreamerStatus{Name: "account", Connected: true}},
		&stubStreamer{status: streamer.StreamerStatus{Name: "market", Connected: true}},
		&stubReconciler{ok: true, latest: reconciler.Result{RunAt: time.Now().UTC(), Status: reconciler.StatusDriftDetected, MismatchCount: 2}},
	)
	entries := logs.FilterMessage("tt watch heartbeat").All()
	if len(entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(entries))
	}
	ctx := entries[0].ContextMap()
	if ctx["degraded"] != true {
		t.Fatalf("degraded = %v, want true", ctx["degraded"])
	}
	if ctx["reason"] != "reconcile_drift" {
		t.Fatalf("reason = %v, want reconcile_drift", ctx["reason"])
	}
	if ctx["reconcile_mismatches"] != int64(2) {
		t.Fatalf("reconcile_mismatches = %v, want 2", ctx["reconcile_mismatches"])
	}
	if ctx["reconcile_policy"] != "observe" {
		t.Fatalf("reconcile_policy = %v, want observe", ctx["reconcile_policy"])
	}
	if ctx["suppress_confidence_actions"] != false {
		t.Fatalf("suppress_confidence_actions = %v, want false", ctx["suppress_confidence_actions"])
	}
}

func TestLogWatchHeartbeat_PartialState(t *testing.T) {
	setHeartbeatGauges(5, 4)
	log, logs := observedWatchLogger(zapcore.InfoLevel)
	logWatchHeartbeat(log,
		&stubStreamer{status: streamer.StreamerStatus{Name: "account", Connected: true}},
		&stubStreamer{status: streamer.StreamerStatus{Name: "market", Connected: true}},
		&stubReconciler{ok: true, latest: reconciler.Result{RunAt: time.Now().UTC(), Status: reconciler.StatusPartial, ErrorText: "snapshot_write_failed"}},
	)
	entries := logs.FilterMessage("tt watch heartbeat").All()
	if len(entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(entries))
	}
	ctx := entries[0].ContextMap()
	if ctx["reconcile_policy"] != "observe" {
		t.Fatalf("reconcile_policy = %v, want observe", ctx["reconcile_policy"])
	}
	if ctx["degraded"] != true {
		t.Fatalf("degraded = %v, want true", ctx["degraded"])
	}
	if ctx["suppress_confidence_actions"] != false {
		t.Fatalf("suppress_confidence_actions = %v, want false", ctx["suppress_confidence_actions"])
	}
}

func TestLogWatchHeartbeat_ErrorState(t *testing.T) {
	setHeartbeatGauges(5, 4)
	log, logs := observedWatchLogger(zapcore.InfoLevel)
	logWatchHeartbeat(log,
		&stubStreamer{status: streamer.StreamerStatus{Name: "account", Connected: true}},
		&stubStreamer{status: streamer.StreamerStatus{Name: "market", Connected: false, LastError: "auth failed"}},
		&stubReconciler{ok: true, latest: reconciler.Result{RunAt: time.Now().UTC(), Status: reconciler.StatusError, ErrorText: "simulated REST timeout"}},
	)
	entries := logs.FilterMessage("tt watch heartbeat").All()
	if len(entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(entries))
	}
	ctx := entries[0].ContextMap()
	if ctx["market"] != "down" {
		t.Fatalf("market = %v, want down", ctx["market"])
	}
	if ctx["reconcile"] != "error" {
		t.Fatalf("reconcile = %v, want error", ctx["reconcile"])
	}
	if ctx["reason"] != "market,reconcile_error" {
		t.Fatalf("reason = %v, want market,reconcile_error", ctx["reason"])
	}
	if ctx["reconcile_policy"] != "suppress" {
		t.Fatalf("reconcile_policy = %v, want suppress", ctx["reconcile_policy"])
	}
	if ctx["suppress_confidence_actions"] != true {
		t.Fatalf("suppress_confidence_actions = %v, want true", ctx["suppress_confidence_actions"])
	}
}

func TestCurrentWatchReconcileStatus_NoRunYet(t *testing.T) {
	view := currentWatchReconcileStatus(&stubReconciler{})
	if view.Available {
		t.Fatal("Available = true before any reconcile run")
	}
	if view.Status != "not_yet_available" {
		t.Fatalf("Status = %q, want not_yet_available", view.Status)
	}
}
