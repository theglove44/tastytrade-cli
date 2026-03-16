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
	exchange "github.com/theglove44/tastytrade-cli/internal/exchange"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
)

type gateTestExchange struct {
	dryRunCalled bool
	submitCalled bool
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

func (g *gateTestExchange) Submit(context.Context, string, models.NewOrder, string) (models.SubmitResult, error) {
	g.submitCalled = true
	return models.SubmitResult{
		Order: models.Order{
			ID:          "submit-1",
			Status:      "Routed",
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
	origFlagSubmitFile, origFlagSubmitYes, origFlagDryRunFile := flagSubmitFile, flagSubmitYes, flagDryRunFile
	origSubmitConfirmIn := submitConfirmIn
	origTransportApproval := isApprovedLiveSubmitTransport
	t.Cleanup(func() {
		cfg, cl, ex, rec, logger = origCfg, origCl, origEx, origRec, origLogger
		flagJSON, readFile = origFlagJSON, origReadFile
		flagSubmitFile, flagSubmitYes, flagDryRunFile = origFlagSubmitFile, origFlagSubmitYes, origFlagDryRunFile
		submitConfirmIn = origSubmitConfirmIn
		isApprovedLiveSubmitTransport = origTransportApproval
	})

	t.Setenv("HOME", t.TempDir())
	cfg = &config.Config{BaseURL: config.ProdBaseURL, AccountID: "TEST123", RateLimits: config.DefaultRateLimits()}
	log, logs := observedDecisionGateLogger()
	logger = log
	cl = client.New(cfg, logger)
	ex = &gateTestExchange{}
	flagJSON = true
	flagSubmitFile = "order.json"
	flagSubmitYes = false
	flagDryRunFile = "order.json"
	submitConfirmIn = strings.NewReader("")
	readFile = func(string) ([]byte, error) {
		return []byte(`{"order-type":"Limit","time-in-force":"Day","price":"1.00","price-effect":"Debit","legs":[{"instrument-type":"Equity","symbol":"AAPL","quantity":1,"action":"Buy to Open"}]}`), nil
	}
	isApprovedLiveSubmitTransport = func(_ exchange.Exchange, _ *config.Config) bool { return true }
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

func TestRunSubmit_BlockedWhenReconcilePolicySuppressesConfidence(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = false
	submitConfirmIn = strings.NewReader("submit\n")
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusError, ErrorText: "rest timeout"}}

	stdout := captureStdout(t, func() {
		err := runSubmit(context.Background())
		if err == nil {
			t.Fatal("runSubmit error = nil, want blocked error")
		}
		if !strings.Contains(err.Error(), "submit blocked by reconcile policy") {
			t.Fatalf("error = %q, want reconcile policy block", err.Error())
		}
	})
	if gx.submitCalled {
		t.Fatal("exchange Submit called despite blocked decision gate")
	}
	if !strings.Contains(stdout, "✗ submit blocked by reconcile policy") {
		t.Fatalf("stdout = %q, want explicit block message", stdout)
	}
	entries := logs.FilterMessage("decision gate: blocked confidence-dependent action").All()
	if len(entries) != 1 {
		t.Fatalf("blocked logs = %d, want 1", len(entries))
	}
}

func TestRunSubmit_JSONRequiresYesFlag(t *testing.T) {
	gx, _ := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = true
	flagSubmitYes = false
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}

	err := runSubmit(context.Background())
	if err == nil {
		t.Fatal("runSubmit error = nil, want acknowledgement block")
	}
	if !strings.Contains(err.Error(), "--json mode requires --yes") {
		t.Fatalf("error = %q, want --yes requirement", err.Error())
	}
	if gx.submitCalled {
		t.Fatal("exchange Submit called without --yes acknowledgement")
	}
}

func TestRunSubmit_OKJSON(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = true
	flagSubmitYes = true
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}

	stdout := captureStdout(t, func() {
		if err := runSubmit(context.Background()); err != nil {
			t.Fatalf("runSubmit: %v", err)
		}
	})
	if !gx.submitCalled {
		t.Fatal("exchange Submit was not called")
	}
	if !strings.Contains(stdout, "\"submitted\": true") || !strings.Contains(stdout, "\"decision_gate_status\": \"allowed\"") {
		t.Fatalf("stdout = %q, want stable submit json output", stdout)
	}
	entries := logs.FilterMessage("decision gate: confidence check passed").All()
	if len(entries) != 1 {
		t.Fatalf("allow logs = %d, want 1", len(entries))
	}
}

func TestRunSubmit_DecliningConfirmationAbortsSafely(t *testing.T) {
	gx, _ := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = false
	submitConfirmIn = strings.NewReader("no\n")
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}

	stdout := captureStdout(t, func() {
		err := runSubmit(context.Background())
		if err == nil {
			t.Fatal("runSubmit error = nil, want operator decline")
		}
		if !strings.Contains(err.Error(), "operator declined confirmation") {
			t.Fatalf("error = %q, want operator decline", err.Error())
		}
	})
	if gx.submitCalled {
		t.Fatal("exchange Submit called despite declined confirmation")
	}
	if !strings.Contains(stdout, "LIVE ORDER SUBMISSION") || !strings.Contains(stdout, "submit declined by operator") {
		t.Fatalf("stdout = %q, want confirmation prompt and decline output", stdout)
	}
}

func TestRunSubmit_AcceptingConfirmationProceeds(t *testing.T) {
	gx, _ := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = false
	submitConfirmIn = strings.NewReader("submit\n")
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}

	stdout := captureStdout(t, func() {
		if err := runSubmit(context.Background()); err != nil {
			t.Fatalf("runSubmit: %v", err)
		}
	})
	if !gx.submitCalled {
		t.Fatal("exchange Submit was not called")
	}
	if !strings.Contains(stdout, "LIVE ORDER SUBMISSION") || !strings.Contains(stdout, "Proceeding with live submission...") || !strings.Contains(stdout, "✓ ORDER SUBMITTED") {
		t.Fatalf("stdout = %q, want prompt, proceed message, and success output", stdout)
	}
}

func TestRunSubmit_AllowsWithWarningWhenReconcileDegraded(t *testing.T) {
	gx, logs := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = false
	submitConfirmIn = strings.NewReader("submit\n")
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusPartial, MismatchCount: 1}}

	stdout := captureStdout(t, func() {
		if err := runSubmit(context.Background()); err != nil {
			t.Fatalf("runSubmit: %v", err)
		}
	})
	if !gx.submitCalled {
		t.Fatal("exchange Submit was not called")
	}
	if !strings.Contains(stdout, "⚠ submit proceeding with degraded reconcile confidence") {
		t.Fatalf("stdout = %q, want explicit warning", stdout)
	}
	if !strings.Contains(stdout, "✓ ORDER SUBMITTED") {
		t.Fatalf("stdout = %q, want submit success output", stdout)
	}
	entries := logs.FilterMessage("decision gate: proceeding with degraded confidence").All()
	if len(entries) != 1 {
		t.Fatalf("warning logs = %d, want 1", len(entries))
	}
}

func TestRunSubmit_RendersPreSubmitPolicyDiagnostics(t *testing.T) {
	gx, _ := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = true
	flagJSON = false
	submitConfirmIn = strings.NewReader("submit\n")
	isApprovedLiveSubmitTransport = func(_ exchange.Exchange, _ *config.Config) bool { return false }
	rec = &stubReconciler{ok: true, latest: reconciler.Result{Status: reconciler.StatusOK}}

	stdout := captureStdout(t, func() {
		err := runSubmit(context.Background())
		if err == nil {
			t.Fatal("runSubmit error = nil, want pre-submit policy denial")
		}
		if !strings.Contains(err.Error(), "submit blocked by pre-submit policy") {
			t.Fatalf("error = %q, want pre-submit denial", err.Error())
		}
	})
	if gx.submitCalled {
		t.Fatal("exchange Submit called despite pre-submit policy denial")
	}
	for _, want := range []string{
		"LIVE SUBMIT DENIED",
		"outcome=deny primary_reason=transport_not_approved",
		"payload_hash_matched=true",
		"approval_freshness=fresh",
		"confirmation_freshness=fresh",
		"duplicate_state=not_checked",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, missing %q", stdout, want)
		}
	}
}

func TestRunSubmit_BlockedWhenLiveTradingDisabled(t *testing.T) {
	gx, _ := setupDecisionGateCommandTest(t)
	cfg.LiveTrading = false
	submitConfirmIn = strings.NewReader("submit\n")

	err := runSubmit(context.Background())
	if err == nil {
		t.Fatal("runSubmit error = nil, want live-trading block")
	}
	if !strings.Contains(err.Error(), "submit blocked: live trading is not enabled") {
		t.Fatalf("error = %q, want live-trading block", err.Error())
	}
	if gx.submitCalled {
		t.Fatal("exchange Submit called while live trading disabled")
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
