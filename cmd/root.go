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
//
// Shutdown order (streamers running):
//
//  1. Context cancelled → account + market streamers exit their Start() loops.
//  2. Shutdown goroutine calls Close() on all four event buses.
//  3. Bus Close() closes subscriber channels → consumer range loops drain and exit.
//  4. wg.Wait() blocks until all four consumers have returned.
//  5. st.Close() flushes and closes SQLite — safe because all writers have exited.
package cmd

import (
	"context"
	"fmt"
	"math/bits"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/config"
	"github.com/theglove44/tastytrade-cli/internal/bus"
	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/exchange"
	ttexchange "github.com/theglove44/tastytrade-cli/internal/exchange/tastytrade"
	"github.com/theglove44/tastytrade-cli/internal/metrics"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

var (
	cfg         *config.Config
	cl          *client.Client
	ex          exchange.Exchange
	rec         reconciler.Reconciler
	acctRuntime streamer.Streamer
	mktRuntime  streamer.MarketStreamer
	st          store.Store
	book        *valuation.MarkBook
	logger      *zap.Logger

	flagJSON       bool
	flagVerbose    bool
	flagNoStreamer bool

	// watchShutdown is initialised in PersistentPreRunE for the watch command
	// before any runtime goroutines are started. shutdown() closes it after all
	// consumers/reconciler/store teardown has completed, allowing watchCmd.RunE
	// to block until clean shutdown is finished.
	watchShutdown chan struct{}
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

		if cmd.Name() == "watch" {
			watchShutdown = make(chan struct{})
		} else {
			watchShutdown = nil
		}

		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		cl = client.New(cfg, logger)
		ex = ttexchange.New(cl, cfg.BaseURL)
		rec = nil
		acctRuntime = nil
		mktRuntime = nil
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
			go func() {
				<-cmd.Context().Done()
				if st != nil {
					if cerr := st.Close(); cerr != nil {
						logger.Warn("store close error", zap.Error(cerr))
					}
				}
				if watchShutdown != nil {
					close(watchShutdown)
				}
			}()
			return nil
		}

		// ── Event buses ───────────────────────────────────────────────────────
		// Each bus is constructed with an onDrop callback that:
		//   (a) increments the BusDroppedEvents Prometheus counter (always), and
		//   (b) emits a zap.Warn log at power-of-2 drop counts to avoid log spam
		//       while still surfacing the condition: 1st, 2nd, 4th, 8th … drop.
		//
		// The rate-limited log uses a shared atomic counter per bus name so the
		// first drop always logs immediately, but subsequent drops log only at
		// doubling intervals. This gives immediate visibility without flooding
		// the log during a persistent back-pressure episode.
		orderBus := bus.New[models.OrderEvent](
			makeDropHandler("order", logger))
		balanceBus := bus.New[models.BalanceEvent](
			makeDropHandler("balance", logger))
		positionBus := bus.New[models.PositionEvent](
			makeDropHandler("position", logger))
		quoteBus := bus.New[models.QuoteEvent](
			makeDropHandler("quote", logger))

		// ── Startup position seeding ──────────────────────────────────────────
		var initSyms []string
		if cfg.AccountID != "" {
			initSyms = seedMarkBookFromREST(cmd.Context(), ex, book, cfg.AccountID, logger)
		}

		// ── Market streamer ───────────────────────────────────────────────────
		quoteHandler := newQuotePublisher(quoteBus, logger)
		mktStreamer := streamer.NewMarketStreamer(
			cfg.DXLinkURL,
			initSyms,
			ex,
			quoteHandler,
			logger,
		)
		mktRuntime = mktStreamer

		go func() {
			if serr := mktStreamer.Start(cmd.Context()); serr != nil {
				logger.Info("market streamer exited", zap.Error(serr))
			}
		}()

		// ── Consumer goroutines with drain WaitGroup ──────────────────────────
		// wg tracks the four event-bus consumer goroutines plus the reconciler.
		// Shutdown order:
		//   1. Bus.Close() → subscriber channels close → range loops exit.
		//   2. wg.Wait() → all consumers AND the reconciler have exited.
		//   3. st.Close() → safe: no writer goroutine is alive.
		var wg sync.WaitGroup
		wg.Add(4)

		go orderConsumer(orderBus.Subscribe(128), st, logger, &wg)
		go balanceConsumer(balanceBus.Subscribe(64), st, logger, &wg)
		go positionConsumer(positionBus.Subscribe(128), book, mktStreamer, logger, &wg)
		go quoteConsumer(quoteBus.Subscribe(256), book, logger, &wg)

		// ── Reconciler ────────────────────────────────────────────────────────
		// Start the reconciler only when an account ID is configured.
		// It runs in its own goroutine, tracked by wg, and stops cleanly when
		// ctx is cancelled (the same context that stops the streamers).
		if cfg.AccountID != "" {
			wg.Add(1)
			rec = reconciler.New(
				ex,
				st,
				book,
				cfg.AccountID,
				reconciler.Config{
					Interval:         cfg.ReconcileInterval,
					AbsenceThreshold: cfg.ReconcileAbsenceThreshold,
				},
				logger,
			)
			go func() {
				defer wg.Done()
				rec.Start(cmd.Context())
			}()
		}

		// ── Account streamer ──────────────────────────────────────────────────
		acctPublisher := newAccountPublisher(orderBus, balanceBus, positionBus, logger)

		shutdown := func() {
			// Step 1: close all buses — signals consumers to drain and exit.
			orderBus.Close()
			balanceBus.Close()
			positionBus.Close()
			quoteBus.Close()
			// Step 2: wait for every consumer to finish draining.
			wg.Wait()
			// Step 3: close the store — all writers have exited.
			if cerr := st.Close(); cerr != nil {
				logger.Warn("store close error", zap.Error(cerr))
			}
			// Step 4: signal the watch command (if running) that shutdown is complete.
			if watchShutdown != nil {
				close(watchShutdown)
			}
		}

		if cfg.AccountID != "" {
			acctStreamer := streamer.NewAccountStreamer(
				cfg.AccountStreamerURL,
				cfg.AccountID,
				cl,
				acctPublisher,
				logger,
			)
			acctRuntime = acctStreamer
			go func() {
				if serr := acctStreamer.Start(cmd.Context()); serr != nil {
					logger.Info("account streamer exited", zap.Error(serr))
				}
				shutdown()
			}()
		} else {
			go func() {
				<-cmd.Context().Done()
				shutdown()
			}()
		}

		return nil
	},
}

// makeDropHandler returns an onDrop callback for a named bus.
// It increments the BusDroppedEvents Prometheus counter on every drop and
// emits a zap.Warn at power-of-2 drop counts (1, 2, 4, 8, …) to make drops
// immediately visible without producing one log line per event.
//
// Using power-of-2 thresholds means:
//   - The very first drop always logs (critical for order/position buses).
//   - A sustained back-pressure episode produces O(log N) log lines, not O(N).
//   - The interval between log lines grows, giving an operator time to act.
func makeDropHandler(busName string, log *zap.Logger) func() {
	var count atomic.Int64
	return func() {
		client.Metrics.BusDroppedEvents.WithLabelValues(busName).Inc()
		n := count.Add(1)
		// Log at 1, 2, 4, 8, 16 … (when n is a power of two).
		// bits.OnesCount64 == 1 iff n is a power of two (and n > 0).
		if bits.OnesCount64(uint64(n)) == 1 {
			log.Warn("bus: event dropped — subscriber channel full",
				zap.String("bus", busName),
				zap.Int64("total_dropped", n),
			)
		}
	}
}

// ── Consumer goroutines ───────────────────────────────────────────────────────
// Each consumer calls wg.Done() via defer when its range loop exits (i.e. when
// its bus channel is closed and fully drained). This guarantees the WaitGroup
// is decremented exactly once per consumer regardless of how the loop exits.

// orderConsumer drains the order event channel and persists confirmed fills.
func orderConsumer(ch <-chan models.OrderEvent, st store.Store, log *zap.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	for ev := range ch {
		if ev.Status != "Filled" {
			continue
		}

		symbol, action, qty, price := "", "", "0", "0"
		if len(ev.Legs) > 0 {
			leg := ev.Legs[0]
			symbol = leg.Symbol
			action = leg.Action
			qty = leg.FillQuantity.String()
			price = leg.FillPrice.String()
		}

		filledAt := ev.FilledAt
		if filledAt == nil {
			log.Warn("orderConsumer: Filled status but nil FilledAt — using now",
				zap.String("order_id", ev.OrderID))
			now := clock()
			filledAt = &now
		}

		rec := store.FillRecord{
			OrderID:       ev.OrderID,
			AccountNumber: ev.AccountNumber,
			Symbol:        symbol,
			Action:        action,
			Quantity:      qty,
			FillPrice:     price,
			FilledAt:      *filledAt,
			Strategy:      "",
			Source:        store.SourceStreamer,
		}
		if err := st.WriteFill(context.Background(), rec); err != nil {
			log.Error("orderConsumer: WriteFill failed",
				zap.String("order_id", ev.OrderID),
				zap.Error(err),
			)
			continue
		}
		client.Metrics.OrdersFilled.WithLabelValues("").Inc()
		log.Info("fill persisted",
			zap.String("order_id", ev.OrderID),
			zap.String("symbol", symbol),
		)
	}
}

// balanceConsumer drains the balance event channel and persists balance records.
func balanceConsumer(ch <-chan models.BalanceEvent, st store.Store, log *zap.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	for ev := range ch {
		rec := store.BalanceRecord{
			AccountNumber:       ev.AccountNumber,
			NetLiquidatingValue: ev.NetLiquidatingValue.String(),
			BuyingPower:         ev.BuyingPower.String(),
			UpdatedAt:           ev.UpdatedAt,
			Source:              store.SourceStreamer,
		}
		if err := st.WriteBalance(context.Background(), rec); err != nil {
			log.Error("balanceConsumer: WriteBalance failed",
				zap.String("account", ev.AccountNumber),
				zap.Error(err),
			)
			continue
		}
		nlq, _ := ev.NetLiquidatingValue.Float64()
		client.Metrics.NLQDollars.Set(nlq)
		log.Debug("balance updated",
			zap.String("nlq", ev.NetLiquidatingValue.String()),
			zap.String("buying_power", ev.BuyingPower.String()),
		)
	}
}

// positionConsumer drains the position event channel and applies changes to
// the MarkBook and market streamer subscription set.
func positionConsumer(
	ch <-chan models.PositionEvent,
	book *valuation.MarkBook,
	mktStreamer streamer.MarketStreamer,
	log *zap.Logger,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	for ev := range ch {
		switch ev.Action {
		case "Open", "Change":
			book.LoadPosition(
				ev.Symbol,
				ev.AccountNumber,
				ev.Quantity.String(),
				ev.QuantityDirection,
				decimal.Zero,
			)
			if mktStreamer != nil {
				marketSymbol := normalizeMarketDataSymbol(ev.Symbol, ev.InstrumentType)
				mktStreamer.Subscribe(marketSymbol)
				if marketSymbol != ev.Symbol && log.Core().Enabled(zap.DebugLevel) {
					log.Debug("position symbol normalized for market data",
						zap.String("symbol", ev.Symbol),
						zap.String("market_symbol", marketSymbol),
					)
				}
			}
			if ev.Action == "Open" {
				client.Metrics.OpenPositions.Inc()
			}
			log.Debug("position opened/changed",
				zap.String("symbol", ev.Symbol),
				zap.String("action", ev.Action),
				zap.String("qty", ev.Quantity.String()),
				zap.String("direction", ev.QuantityDirection),
			)

		case "Close":
			book.RemovePosition(ev.Symbol)
			client.Metrics.OpenPositions.Dec()
			log.Debug("position closed", zap.String("symbol", ev.Symbol))
		}
	}
}

// quoteConsumer drains the quote event channel and applies each quote to the
// MarkBook, updating Prometheus metrics.
func quoteConsumer(ch <-chan models.QuoteEvent, book *valuation.MarkBook, log *zap.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	for ev := range ch {
		snap := book.ApplyQuote(
			ev.Symbol,
			ev.BidPrice,
			ev.AskPrice,
			ev.LastPrice,
			ev.MarkPrice,
			ev.MarkStale,
			ev.EventTime,
		)
		client.Metrics.QuotesReceived.WithLabelValues(ev.Symbol).Inc()
		client.Metrics.LastQuoteTime.SetToCurrentTime()

		if log.Core().Enabled(zap.DebugLevel) {
			log.Debug("quote applied",
				zap.String("symbol", ev.Symbol),
				zap.String("mark", snap.MarkPrice.String()),
				zap.Bool("stale", snap.MarkStale),
				zap.String("unrealized_pnl", snap.UnrealizedPnL.String()),
			)
		}
	}
}

// ── seedMarkBookFromREST ──────────────────────────────────────────────────────

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
		avgOpen := p.AverageOpenPrice
		if avgOpen.IsZero() {
			avgOpen = p.ClosePrice
		}
		book.LoadPosition(
			p.Symbol,
			p.AccountNumber,
			p.Quantity.String(),
			p.QuantityDirection,
			avgOpen,
		)
		syms = append(syms, normalizeMarketDataSymbol(p.Symbol, p.InstrumentType))
	}

	client.Metrics.OpenPositions.Set(float64(len(positions)))
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
		brokerOrdersCmd,
		submitCmd,
		dryRunCmd,
		killCmd,
		resumeCmd,
		watchCmd,
		submitStateCmd,
	)
}
