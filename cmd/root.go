// Package cmd implements the CLI commands for tastytrade-cli.
//
// Command tree:
//
//	tt login              — OAuth flow, store credentials in keychain
//	tt accounts           — list accounts
//	tt positions [--json] — list open positions
//	tt orders   [--json]  — list live orders
//	tt dry-run  [--json]  — simulate an order (dry-run only, never submits)
//	tt kill               — arm the file-based kill switch
//	tt resume             — disarm the file-based kill switch
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/exchange"
	ttexchange "github.com/theglove44/tastytrade-cli/internal/exchange/tastytrade"
	"github.com/theglove44/tastytrade-cli/internal/metrics"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
	"go.uber.org/zap"
)

var (
	cfg    *config.Config
	cl     *client.Client
	ex     exchange.Exchange // commands call ex, not cl directly
	st     store.Store       // nil when store is disabled (e.g. login command)
	logger *zap.Logger

	// global flags
	flagJSON           bool
	flagVerbose        bool
	flagNoStreamer      bool // --no-streamer: skip account streamer startup
)

var rootCmd = &cobra.Command{
	Use:   "tt",
	Short: "TastyTrade CLI — positions, orders, and automation pipeline tooling",
	Long: `tt is a TastyTrade API client designed for automation pipeline use.
All credentials are stored in the OS keychain. Run 'tt login' first.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config+client init for commands that don't need them.
		switch cmd.Name() {
		case "login", "kill", "resume":
			return nil
		}

		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		cl = client.New(cfg, logger)
		ex = ttexchange.New(cl, cfg.BaseURL)

		// Start the Prometheus /metrics server (background goroutine).
		// Binds to 127.0.0.1 only. Address from TASTYTRADE_METRICS_ADDR or :9090.
		metricsAddr := metrics.Addr(logger)
		metrics.Serve(cmd.Context(), metricsAddr, logger)

		// Open the SQLite store. Failure is non-fatal: log and continue without
		// persistence. The streamer will not be started if the store fails.
		var storeErr error
		st, storeErr = store.Open(logger)
		if storeErr != nil {
			logger.Warn("store unavailable — persistence and streamer disabled",
				zap.Error(storeErr))
		}

		// Start the account streamer in a background goroutine unless:
		//   - --no-streamer flag is set
		//   - store failed to open (no point streaming without persistence)
		//   - no account ID configured
		if !flagNoStreamer && st != nil && cfg.AccountID != "" {
			handler := newAccountEventHandler(st, logger)
			acctStreamer := streamer.NewAccountStreamer(
				cfg.AccountStreamerURL,
				cfg.AccountID,
				cl,
				handler,
				logger,
			)
			go func() {
				if err := acctStreamer.Start(cmd.Context()); err != nil {
					logger.Info("account streamer exited", zap.Error(err))
				}
				// Close store when streamer exits (context cancelled = command done).
				if closeErr := st.Close(); closeErr != nil {
					logger.Warn("store close error", zap.Error(closeErr))
				}
			}()
		} else if st != nil && (flagNoStreamer || cfg.AccountID == "") {
			// Store opened but streamer skipped — close store when command exits.
			go func() {
				<-cmd.Context().Done()
				if closeErr := st.Close(); closeErr != nil {
					logger.Warn("store close error", zap.Error(closeErr))
				}
			}()
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	var logErr error
	// Default to production logger; switch to development (pretty) with --verbose.
	cobra.OnInitialize(func() {
		if flagVerbose {
			logger, logErr = zap.NewDevelopment()
		} else {
			logger, logErr = zap.NewProduction()
		}
		if logErr != nil {
			fmt.Fprintf(os.Stderr, "logger init: %v\n", logErr)
			os.Exit(1)
		}
	})

	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false,
		"Output as stable JSON (for automation pipeline consumption)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false,
		"Development-mode logging (human-readable, debug level)")
	rootCmd.PersistentFlags().BoolVar(&flagNoStreamer, "no-streamer", false,
		"Skip account streamer startup (useful for one-shot commands in scripts)")

	rootCmd.AddCommand(
		loginCmd,
		accountsCmd,
		positionsCmd,
		ordersCmd,
		dryRunCmd,
		killCmd,
		resumeCmd,
	)
}
