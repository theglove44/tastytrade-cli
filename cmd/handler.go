package cmd

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/store"
)

// accountEventHandler implements streamer.AccountHandler.
// It bridges account streamer events to:
//   - store writes (fills, balances, position counts)
//   - Prometheus metrics updates (OrdersFilled, NLQDollars, OpenPositions)
//
// All handler methods are called from the streamer's dispatch goroutine.
// They must not block; any slow work is delegated to a goroutine.
type accountEventHandler struct {
	st  store.Store
	log *zap.Logger
}

// newAccountEventHandler creates a handler backed by the given store.
func newAccountEventHandler(st store.Store, log *zap.Logger) *accountEventHandler {
	return &accountEventHandler{st: st, log: log}
}

// OnOrderEvent handles order status changes from the account streamer.
// Confirmed fills (Status=="Filled") are persisted to the store and counted
// in the OrdersFilled metric.
func (h *accountEventHandler) OnOrderEvent(ev models.OrderEvent) {
	if ev.Status != "Filled" {
		return
	}

	// Build a fill record for the first leg (representative symbol).
	// Multi-leg fills: all legs share the same OrderID so the store's
	// idempotency check correctly deduplicates on reconnect snapshots.
	symbol := ""
	action := ""
	qty := "0"
	price := "0"
	if len(ev.Legs) > 0 {
		leg := ev.Legs[0]
		symbol = leg.Symbol
		action = leg.Action
		qty = leg.FillQuantity.String()
		price = leg.FillPrice.String()
	}

	filledAt := ev.FilledAt
	if filledAt == nil {
		h.log.Warn("OnOrderEvent: Filled status but nil FilledAt — using now",
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
		Strategy:      "", // backfilled by reconciliation pass against intent log
		Source:        store.SourceStreamer,
	}

	// Persist asynchronously — must not block the dispatch goroutine.
	go func() {
		if err := h.st.WriteFill(context.Background(), rec); err != nil {
			h.log.Error("OnOrderEvent: WriteFill failed",
				zap.String("order_id", ev.OrderID),
				zap.Error(err),
			)
			return
		}
		// Metric: increment OrdersFilled after confirmed persistence.
		// Strategy label is "" until reconciliation enriches it.
		client.Metrics.OrdersFilled.WithLabelValues("").Inc()
		h.log.Info("fill persisted",
			zap.String("order_id", ev.OrderID),
			zap.String("symbol", symbol),
		)
	}()
}

// OnBalanceEvent handles account balance updates from the account streamer.
// Persists the latest balance and updates the NLQDollars metric.
func (h *accountEventHandler) OnBalanceEvent(ev models.BalanceEvent) {
	rec := store.BalanceRecord{
		AccountNumber:       ev.AccountNumber,
		NetLiquidatingValue: ev.NetLiquidatingValue.String(),
		BuyingPower:         ev.BuyingPower.String(),
		UpdatedAt:           ev.UpdatedAt,
		Source:              store.SourceStreamer,
	}

	go func() {
		if err := h.st.WriteBalance(context.Background(), rec); err != nil {
			h.log.Error("OnBalanceEvent: WriteBalance failed",
				zap.String("account", ev.AccountNumber),
				zap.Error(err),
			)
			return
		}
		nlq, _ := ev.NetLiquidatingValue.Float64()
		client.Metrics.NLQDollars.Set(nlq)
		h.log.Debug("balance updated",
			zap.String("nlq", ev.NetLiquidatingValue.String()),
			zap.String("buying_power", ev.BuyingPower.String()),
		)
	}()
}

// OnPositionEvent handles position changes from the account streamer.
// Updates the OpenPositions gauge. Position snapshots are written by the
// REST poller (Phase 2B); the streamer only updates the live count metric.
func (h *accountEventHandler) OnPositionEvent(ev models.PositionEvent) {
	// OpenPositions gauge: incremented on Open, decremented on Close.
	// "Change" events do not affect the count.
	switch ev.Action {
	case "Open":
		client.Metrics.OpenPositions.Inc()
	case "Close":
		client.Metrics.OpenPositions.Dec()
	}
	h.log.Debug("position event",
		zap.String("symbol", ev.Symbol),
		zap.String("action", ev.Action),
	)
}

// clock is a variable to allow test injection of time.Now.
var clock = func() time.Time { return time.Now() }
