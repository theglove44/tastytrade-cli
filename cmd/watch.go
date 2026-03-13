package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	imetrics "github.com/theglove44/tastytrade-cli/internal/metrics"
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
