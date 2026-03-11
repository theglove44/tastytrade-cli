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
	"go.uber.org/zap"
)

var (
	cfg    *config.Config
	cl     *client.Client
	ex     exchange.Exchange // commands call ex, not cl directly
	logger *zap.Logger

	// global flags
	flagJSON    bool
	flagVerbose bool
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
