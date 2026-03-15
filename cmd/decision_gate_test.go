package cmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

type gateTestExchange struct {
	dryRunCalled bool
	ordersCalled bool
}

func (g *gateTestExchange) Accounts(context.Context) ([]models.Account, error) {
	return nil, nil
}

func (g *gateTestExchange) Positions(context.Context, string) ([]models.Position, error) {
	return nil, nil
}

func (g *gateTestExchange) Orders(context.Context, string) ([]models.Order, error) {
	g.ordersCalled = true
	return nil, nil
}

func (g *gateTestExchange) DryRun(context.Context, string, models.NewOrder, string) (models.DryRunResult, error) {
	g.dryRunCalled = true
	return models.DryRunResult{
		Order: models.Order{
			ID:          "dry-run-1",
			Status:      "Received",
			OrderType:   "Limit",
			TimeInForce: "Day",
			Price:       decimal.RequireFromString("1.00"),
			PriceEffect: "Debit",
		},
		BuyingPowerEffect: models.BPEffect{
			ChangeInBuyingPower:             decimal.RequireFromString("-100.00"),
			ChangeInBuyingPowerEffect:       "Debit",
			ChangeInMarginRequirement:       decimal.RequireFromString("0"),
			ChangeInMarginRequirementEffect: "None",
			CurrentBuyingPower:              decimal.RequireFromString("1000.00"),
			NewBuyingPower:                  decimal.RequireFromString("900.00"),
		},
	}, nil
}

func (g *gateTestExchange) QuoteToken(context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

func observedDecisionGateLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.InfoLevel)
	return zap.New(core), logs
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(data)
}

func setupDecisionGateCommandTest(t *testing.T) (*gateTestExchange, *observer.ObservedLogs) {
	t.Helper()
	origCfg, origCl, origEx, origRec, origLogger := cfg, cl, ex, rec, logger
	origFlagJSON, origReadFile := flagJSON, readFile
	t.Cleanup(func() {
		cfg, cl, ex, rec, logger = origCfg, origCl, origEx, origRec, origLogger
		flagJSON, readFile = origFlagJSON, origReadFile
	})

	t.Setenv("HOME", t.TempDir())
	cfg = &config.Config{AccountID: "TEST123", RateLimits: config.DefaultRateLimits()}
	log, logs := observedDecisionGateLogger()
	logger = log
	cl = client.New(cfg, logger)
	ex = &gateTestExchange{}
	flagJSON = true
	readFile = func(string) ([]byte, error) {
		return []byte(`{"order-type":"Limit","time-in-force":"Day","price":"1.00","price-effect":"Debit","legs":[{"instrument-type":"Equity","symbol":"AAPL","quantity":1,"action":"Buy to Open"}]}`), nil
	}
	return ex.(*gateTestExchange), logs
}

func TestEnforceDecisionGate_OKPasses(t *testing.T) {
	log, logs := observedDecisionGateLogger()
	err := enforceDecisionGate("dry-run", &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}, log)
	if err != nil {
		t.Fatalf("enforceDecisionGate: %v", err)
	}
	entries := logs.FilterMessage("decision gate: confidence check passed").All()
	if len(entries) != 1 {
		t.Fatalf("confidence check logs = %d, want 1", len(entries))
	}
}

func TestEnforceDecisionGate_DriftWarns(t *testing.T) {
	log, logs := observedDecisionGateLogger()
	err := enforceDecisionGate("dry-run", &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusDriftDetected}}, log)
	if err != nil {
		t.Fatalf("enforceDecisionGate: %v", err)
	}
	entries := logs.FilterMessage("decision gate: proceeding with degraded confidence").All()
	if len(entries) != 1 {
		t.Fatalf("warning logs = %d, want 1", len(entries))
	}
	ctx := entries[0].ContextMap()
	if ctx["gate_outcome"] != "allowed_with_warning" {
		t.Fatalf("gate_outcome = %v, want allowed_with_warning", ctx["gate_outcome"])
	}
	if ctx["reconcile_status"] != "drift_detected" {
		t.Fatalf("reconcile_status = %v, want drift_detected", ctx["reconcile_status"])
	}
}

func TestEnforceDecisionGate_PartialWarns(t *testing.T) {
	log, logs := observedDecisionGateLogger()
	err := enforceDecisionGate("dry-run", &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusPartial}}, log)
	if err != nil {
		t.Fatalf("enforceDecisionGate: %v", err)
	}
	entries := logs.FilterMessage("decision gate: proceeding with degraded confidence").All()
	if len(entries) != 1 {
		t.Fatalf("warning logs = %d, want 1", len(entries))
	}
	ctx := entries[0].ContextMap()
	if ctx["reconcile_status"] != "partial" {
		t.Fatalf("reconcile_status = %v, want partial", ctx["reconcile_status"])
	}
	if ctx["suppress_confidence_actions"] != false {
		t.Fatalf("suppress_confidence_actions = %v, want false", ctx["suppress_confidence_actions"])
	}
}

func TestRunDryRun_BlockedWhenReconcilePolicySuppressesConfidence(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusError, ErrorText: "rest timeout"}}

	err := runDryRun(context.Background())
	if err == nil {
		t.Fatal("runDryRun error = nil, want blocked error")
	}
	if !strings.Contains(err.Error(), "dry-run blocked by reconcile policy") {
		t.Fatalf("error = %q, want reconcile policy block", err.Error())
	}
	if !strings.Contains(err.Error(), "status=error") || !strings.Contains(err.Error(), "policy=suppress") {
		t.Fatalf("error = %q, want status/policy detail", err.Error())
	}
	if gx.dryRunCalled {
		t.Fatal("exchange DryRun called despite blocked decision gate")
	}
	entries := logs.FilterMessage("decision gate: blocked confidence-dependent action").All()
	if len(entries) != 1 {
		t.Fatalf("blocked logs = %d, want 1", len(entries))
	}
}

func TestRunDryRun_AllowsWithWarningWhenReconcileDegraded(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusDriftDetected, MismatchCount: 1}}

	stdout := captureStdout(t, func() {
		if err := runDryRun(context.Background()); err != nil {
			t.Fatalf("runDryRun: %v", err)
		}
	})
	if !gx.dryRunCalled {
		t.Fatal("exchange DryRun was not called")
	}
	if !strings.Contains(stdout, "\"ok\": true") {
		t.Fatalf("stdout = %q, want dry-run json output", stdout)
	}
	entries := logs.FilterMessage("decision gate: proceeding with degraded confidence").All()
	if len(entries) != 1 {
		t.Fatalf("warning logs = %d, want 1", len(entries))
	}
}

func TestRunOrders_UnaffectedByDecisionGate(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusError, ErrorText: "rest timeout"}}

	stdout := captureStdout(t, func() {
		if err := runOrders(context.Background()); err != nil {
			t.Fatalf("runOrders: %v", err)
		}
	})
	if !gx.ordersCalled {
		t.Fatal("exchange Orders was not called")
	}
	if !strings.Contains(stdout, "\"count\": 0") {
		t.Fatalf("stdout = %q, want orders json output", stdout)
	}
	if got := len(logs.FilterMessage("decision gate: blocked confidence-dependent action").All()); got != 0 {
		t.Fatalf("blocked gate logs = %d, want 0 for read-only orders path", got)
	}
}
