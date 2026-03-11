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
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/exchange"
	ttexchange "github.com/theglove44/tastytrade-cli/internal/exchange/tastytrade"
	"github.com/theglove44/tastytrade-cli/internal/metrics"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

var (
	cfg    *config.Config
	cl     *client.Client
	ex     exchange.Exchange
	st     store.Store
	book   *valuation.MarkBook
	logger *zap.Logger

	flagJSON       bool
	flagVerbose    bool
	flagNoStreamer bool
)

var rootCmd = &cobra.Command{
	Use:   "tt",
	Short: "TastyTrade CLI — positions, orders, and automation pipeline tooling",
	Long: `tt is a TastyTrade API client designed for automation pipeline use.
All credentials are stored in the OS keychain. Run 'tt login' first.`,
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
		book = valuation.NewMarkBook()

		metricsAddr := metrics.Addr(logger)
		metrics.Serve(cmd.Context(), metricsAddr, logger)

		var storeErr error
		st, storeErr = store.Open(logger)
		if storeErr != nil {
			logger.Warn("store unavailable — persistence and streamers disabled",
				zap.Error(storeErr))
		}

		if flagNoStreamer || st == nil {
			if st != nil {
				go func() {
					<-cmd.Context().Done()
					if cerr := st.Close(); cerr != nil {
						logger.Warn("store close error", zap.Error(cerr))
					}
				}()
			}
			return nil
		}

		// ── Startup position seeding ─────────────────────────────────────────
		// Fetch open positions via REST before constructing the market streamer,
		// so the initial FEED_SUBSCRIPTION contains all currently-open symbols.
		// This runs synchronously in PersistentPreRunE — it is a single REST
		// call, so the latency hit is acceptable and guarantees the streamer
		// starts with a populated symbol set.
		//
		// Failure is non-fatal: log and continue with an empty MarkBook.
		// The account streamer's OnPositionEvent will backfill as events arrive.
		var initSyms []string
		if cfg.AccountID != "" {
			initSyms = seedMarkBookFromREST(cmd.Context(), ex, book, cfg.AccountID, logger)
		}

		// ── Market streamer ──────────────────────────────────────────────────
		// Constructed first so the account handler can hold a reference to it.
		// The account handler calls mktStreamer.Subscribe() when new positions
		// arrive via OnPositionEvent — Subscribe() is safe to call before and
		// after Start().
		quoteHandler := newQuoteEventHandler(book, logger)
		mktStreamer := streamer.NewMarketStreamer(
			cfg.DXLinkURL,
			initSyms,
			ex,
			quoteHandler,
			logger,
		)

		go func() {
			if serr := mktStreamer.Start(cmd.Context()); serr != nil {
				logger.Info("market streamer exited", zap.Error(serr))
			}
		}()

		// ── Account streamer ─────────────────────────────────────────────────
		if cfg.AccountID != "" {
			acctHandler := newAccountEventHandler(st, book, mktStreamer, logger)
			acctStreamer := streamer.NewAccountStreamer(
				cfg.AccountStreamerURL,
				cfg.AccountID,
				cl,
				acctHandler,
				logger,
			)
			go func() {
				if serr := acctStreamer.Start(cmd.Context()); serr != nil {
					logger.Info("account streamer exited", zap.Error(serr))
				}
				if cerr := st.Close(); cerr != nil {
					logger.Warn("store close error", zap.Error(cerr))
				}
			}()
		} else {
			go func() {
				<-cmd.Context().Done()
				if cerr := st.Close(); cerr != nil {
					logger.Warn("store close error", zap.Error(cerr))
				}
			}()
		}

		return nil
	},
}

// seedMarkBookFromREST fetches open positions for the account and loads them
// into the MarkBook. Returns the list of symbols for the initial market
// streamer subscription.
//
// The REST Position model carries AverageOpenPrice — this is the correct cost
// basis. Using REST seeding gives the valuation layer accurate PnL from the
// moment the market streamer receives its first quote, without waiting for
// a PositionEvent snapshot from the account streamer.
//
// Timeout: 10 seconds. On any error, returns whatever symbols were loaded
// before the failure (partial seeding is better than no seeding).
func seedMarkBookFromREST(
	ctx context.Context,
	ex exchange.Exchange,
	book *valuation.MarkBook,
	accountID string,
	log *zap.Logger,
) []string {
	seedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	positions, err := ex.Positions(seedCtx, accountID)
	if err != nil {
		log.Warn("startup position seeding failed — MarkBook starts empty",
			zap.String("account", accountID),
			zap.Error(err),
		)
		return nil
	}

	syms := make([]string, 0, len(positions))
	for _, p := range positions {
		// Use AverageOpenPrice from REST — this is the true cost basis.
		// The account streamer PositionEvent does not carry this field,
		// so REST seeding is the only way to get it at startup.
		avgOpen := p.AverageOpenPrice
		if avgOpen.IsZero() {
			// Fallback: some instrument types may not populate this field.
			// ClosePrice (prior day) is a reasonable proxy until updated.
			avgOpen = p.ClosePrice
		}
		book.LoadPosition(
			p.Symbol,
			p.AccountNumber,
			p.Quantity.String(),
			p.QuantityDirection,
			avgOpen,
		)
		syms = append(syms, p.Symbol)
	}

	log.Info("startup position seeding complete",
		zap.String("account", accountID),
		zap.Int("positions", len(positions)),
		zap.Strings("symbols", syms),
	)
	return syms
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	var logErr error
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
		"Skip streamer startup (useful for one-shot commands in scripts)")

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
