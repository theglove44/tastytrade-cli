package cmd

import (
	"context"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	imetrics "github.com/theglove44/tastytrade-cli/internal/metrics"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
)

// watchCmd is the long-running runtime mode for tastytrade-cli.
//
// It starts all infrastructure components via PersistentPreRunE in root.go and
// keeps the process alive until the user presses Ctrl+C (SIGINT) or SIGTERM.
//
// Shutdown signaling is safe for Cobra ordering because watchShutdown is
// initialised in root PersistentPreRunE (which runs before RunE), not here.
var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Long-running mode: keep streamers, reconciler, and metrics running",
	Long: `tt watch starts all infrastructure components and keeps the process alive.

No orders are placed. No strategy logic runs. Use this command to:
  - exercise the Phase 2/3A infrastructure against a live account
  - validate streamer connectivity and reconciliation
  - scrape Prometheus metrics

Press Ctrl+C to trigger a clean shutdown.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		metricsAddr := "http://" + imetrics.Addr(logger) + "/metrics"
		logger.Info("tt watch starting",
			zap.String("account", cfg.AccountID),
			zap.Duration("reconcile_interval", cfg.ReconcileInterval),
			zap.Int("absence_threshold", cfg.ReconcileAbsenceThreshold),
			zap.String("metrics", metricsAddr),
			zap.Bool("live_trading", cfg.LiveTrading),
		)

		if cfg.AccountID == "" {
			logger.Warn("TASTYTRADE_ACCOUNT_ID not set — account streamer and reconciler are disabled")
		}

		if killed, reason := client.KillSwitch(); killed {
			logger.Warn("kill switch is ARMED — order submission is blocked", zap.String("reason", reason))
		} else {
			logger.Info("kill switch is disarmed — order submission gates are nominal")
		}

		logWatchHeartbeat(logger, acctRuntime, mktRuntime, rec)
		logWatchReconcileStatus(logger, rec)
		go watchStatusLoop(cmd.Context(), logger, acctRuntime, mktRuntime, rec, cfg.ReconcileInterval)

		logger.Info("tt watch ready — press Ctrl+C to stop")

		<-cmd.Context().Done()
		logger.Info("tt watch: shutdown signal received")

		select {
		case <-watchShutdown:
			logger.Info("tt watch: clean shutdown complete")
		case <-time.After(15 * time.Second):
			logger.Warn("tt watch: shutdown timed out after 15s — forcing exit")
		}

		return nil
	},
}

type watchReconcileStatusView struct {
	Available          bool
	Status             string
	RunAt              time.Time
	Duration           time.Duration
	PositionsChecked   int
	SymbolsChecked     int
	MismatchCount      int
	MismatchCategories map[string]int
	RecoveryTriggered  bool
	Action             string
	ErrorText          string
}

type watchHeartbeatView struct {
	AccountStatus             string
	MarketStatus              string
	ReconcileStatus           string
	ReconcilePolicy           string
	SuppressConfidenceActions bool
	ReconcileLastRunAt        string
	ReconcileMismatches       int
	TrackedSymbols            int
	OpenPositions             int
	Degraded                  bool
	Reason                    string
}

func currentWatchReconcileStatus(rec reconciler.Reconciler) watchReconcileStatusView {
	if rec == nil {
		return watchReconcileStatusView{Status: "not_yet_available"}
	}
	latest, ok := rec.LatestResult()
	if !ok {
		return watchReconcileStatusView{Status: "not_yet_available"}
	}
	return watchReconcileStatusView{
		Available:          true,
		Status:             string(latest.Status),
		RunAt:              latest.RunAt,
		Duration:           latest.Duration,
		PositionsChecked:   latest.PositionsChecked,
		SymbolsChecked:     latest.SymbolsChecked,
		MismatchCount:      latest.MismatchCount,
		MismatchCategories: latest.MismatchCategories,
		RecoveryTriggered:  latest.RecoveryTriggered,
		Action:             latest.Action,
		ErrorText:          latest.ErrorText,
	}
}

func currentWatchHeartbeat(acct streamer.Streamer, mkt streamer.Streamer, rec reconciler.Reconciler) watchHeartbeatView {
	recView := currentWatchReconcileStatus(rec)
	view := watchHeartbeatView{
		AccountStatus:       streamerHealth(acct),
		MarketStatus:        streamerHealth(mkt),
		ReconcileStatus:     recView.Status,
		ReconcilePolicy:     "not_yet_available",
		ReconcileLastRunAt:  "n/a",
		ReconcileMismatches: recView.MismatchCount,
		TrackedSymbols:      metricGaugeInt(client.Metrics.TrackedSymbols),
		OpenPositions:       metricGaugeInt(client.Metrics.OpenPositions),
	}
	if recView.Available {
		policy := reconciler.PolicyForResult(reconciler.Result{Status: reconciler.Status(recView.Status)})
		view.ReconcilePolicy = string(policy.Handling)
		view.SuppressConfidenceActions = policy.SuppressConfidenceActions
		view.ReconcileLastRunAt = recView.RunAt.Format(time.RFC3339)
		if policy.Degraded {
			view.Degraded = true
		}
	}

	var reasons []string
	if view.AccountStatus != "up" && view.AccountStatus != "n/a" {
		reasons = append(reasons, "account")
	}
	if view.MarketStatus != "up" && view.MarketStatus != "n/a" {
		reasons = append(reasons, "market")
	}
	switch recView.Status {
	case string(reconciler.StatusDriftDetected):
		reasons = append(reasons, "reconcile_drift")
	case string(reconciler.StatusPartial):
		reasons = append(reasons, "reconcile_partial")
	case string(reconciler.StatusError):
		reasons = append(reasons, "reconcile_error")
	}
	if len(reasons) > 0 {
		view.Degraded = true
		view.Reason = joinReasons(reasons)
	}
	return view
}

func logWatchHeartbeat(log *zap.Logger, acct streamer.Streamer, mkt streamer.Streamer, rec reconciler.Reconciler) {
	view := currentWatchHeartbeat(acct, mkt, rec)
	fields := []zap.Field{
		zap.String("account", view.AccountStatus),
		zap.String("market", view.MarketStatus),
		zap.String("reconcile", view.ReconcileStatus),
		zap.String("reconcile_policy", view.ReconcilePolicy),
		zap.Bool("suppress_confidence_actions", view.SuppressConfidenceActions),
		zap.String("reconcile_last_run_at", view.ReconcileLastRunAt),
		zap.Int("reconcile_mismatches", view.ReconcileMismatches),
		zap.Int("tracked_symbols", view.TrackedSymbols),
		zap.Int("open_positions", view.OpenPositions),
	}
	if view.Degraded {
		fields = append(fields,
			zap.Bool("degraded", true),
			zap.String("reason", view.Reason),
		)
		log.Warn("tt watch heartbeat", fields...)
		return
	}
	fields = append(fields, zap.Bool("degraded", false))
	log.Info("tt watch heartbeat", fields...)
}

func logWatchReconcileStatus(log *zap.Logger, rec reconciler.Reconciler) {
	view := currentWatchReconcileStatus(rec)
	fields := []zap.Field{
		zap.String("reconcile_status", view.Status),
	}
	if view.Available {
		policy := reconciler.PolicyForResult(reconciler.Result{Status: reconciler.Status(view.Status)})
		fields = append(fields,
			zap.String("reconcile_policy", string(policy.Handling)),
			zap.Bool("reconcile_degraded", policy.Degraded),
			zap.Bool("suppress_confidence_actions", policy.SuppressConfidenceActions),
			zap.Time("reconcile_last_run_at", view.RunAt),
			zap.Duration("reconcile_duration", view.Duration),
			zap.Int("reconcile_positions_checked", view.PositionsChecked),
			zap.Int("reconcile_symbols_checked", view.SymbolsChecked),
			zap.Int("reconcile_mismatch_count", view.MismatchCount),
			zap.Any("reconcile_mismatch_categories", view.MismatchCategories),
			zap.Bool("reconcile_recovery_triggered", view.RecoveryTriggered),
			zap.String("reconcile_action", view.Action),
		)
		if view.ErrorText != "" {
			fields = append(fields, zap.String("reconcile_error", view.ErrorText))
		}
	}
	log.Info("tt watch reconcile status", fields...)
}

func watchStatusLoop(ctx context.Context, log *zap.Logger, acct streamer.Streamer, mkt streamer.Streamer, rec reconciler.Reconciler, every time.Duration) {
	if every <= 0 {
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logWatchHeartbeat(log, acct, mkt, rec)
			logWatchReconcileStatus(log, rec)
		}
	}
}

func streamerHealth(s streamer.Streamer) string {
	if s == nil {
		return "n/a"
	}
	st := s.Status()
	if st.Connected {
		if st.LastError != "" {
			return "degraded"
		}
		return "up"
	}
	if st.LastError != "" {
		return "down"
	}
	return "starting"
}

func metricGaugeInt(g interface{ Write(*dto.Metric) error }) int {
	m := &dto.Metric{}
	if err := g.Write(m); err != nil || m.Gauge == nil || m.Gauge.Value == nil {
		return 0
	}
	return int(m.Gauge.GetValue())
}

func joinReasons(in []string) string {
	if len(in) == 0 {
		return ""
	}
	out := in[0]
	for i := 1; i < len(in); i++ {
		out += "," + in[i]
	}
	return out
}
