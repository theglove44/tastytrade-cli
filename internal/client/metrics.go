package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus instruments for the tastytrade-cli.
//
// Expose via: http.Handle("/metrics", promhttp.Handler())
// Bind to localhost:9090/metrics only (env: TASTYTRADE_METRICS_ADDR).
// The /metrics server itself is started in internal/web (Phase 2).
//
// Wiring status (spec compliance phase):
//
//	Instrument                  Wired?  Updated by
//	──────────────────────────  ──────  ─────────────────────────────────────────
//	OrdersSubmitted             STUB    live-submit cmd (Phase 2)
//	OrdersFilled                STUB    account streamer OnOrderFill (Phase 2)
//	OrderLatency                STUB    account streamer (Phase 2)
//	APIErrors                   ✓       client.Do — all 4xx/5xx paths
//	RateLimitHits               ✓       client.Do — 429 path
//	RequestDuration             ✓       client.Do — every request
//	TokenRefreshes              ✓       auth.doTokenRefresh — ok/fail/missing_refresh_token
//	StreamerReconnects          STUB    streamer.Connect (Phase 2)
//	StreamerUptime              STUB    streamer.Connect (Phase 2)
//	NLQDollars                  STUB    balance poller (Phase 2)
//	OpenPositions               STUB    positions poller (Phase 2)
//	CircuitBreakerState         ✓       circuit_breaker.Allow / Reset
//	KillSwitchState             ✓       killswitch.KillSwitch()
var Metrics = newMetrics()

type metrics struct {
	// Orders
	OrdersSubmitted *prometheus.CounterVec // label: strategy
	OrdersFilled    *prometheus.CounterVec // label: strategy
	OrderLatency    prometheus.Histogram   // time from dry-run to fill event

	// API errors + rate limits
	APIErrors       *prometheus.CounterVec   // label: status_code
	RateLimitHits   *prometheus.CounterVec   // label: family
	RequestDuration *prometheus.HistogramVec // label: family, method

	// Auth
	TokenRefreshes *prometheus.CounterVec // label: outcome (ok, fail)

	// Streamers
	StreamerReconnects *prometheus.CounterVec // label: streamer (account, market)
	StreamerUptime     *prometheus.GaugeVec   // label: streamer — seconds since last connect

	// Account state
	NLQDollars    prometheus.Gauge
	OpenPositions prometheus.Gauge

	// Safety controls
	CircuitBreakerState prometheus.Gauge // 0=normal 1=tripped
	KillSwitchState     prometheus.Gauge // 0=normal 1=halted

	// Market data (Phase 2B)
	// QuotesReceived: total quote events decoded from DXLink FEED_DATA frames.
	QuotesReceived *prometheus.CounterVec // label: symbol
	// TrackedSymbols: number of symbols currently subscribed on the market streamer.
	TrackedSymbols prometheus.Gauge
	// LastQuoteTime: Unix timestamp of the most recently received quote event.
	// Useful for stale-data alerting: alert if now - LastQuoteTime > threshold.
	LastQuoteTime prometheus.Gauge

	// Bus observability (Phase 2C hardening)
	// BusDroppedEvents counts events dropped by the internal bus because the
	// subscriber channel was full at publish time.
	// label: bus — bounded set: order | balance | position | quote
	BusDroppedEvents *prometheus.CounterVec

	// Reconciler observability (Phase 3A)
	ReconcileRunsTotal           prometheus.Counter
	ReconcileErrorsTotal         prometheus.Counter
	ReconcilePositionsCorrected  prometheus.Counter
	ReconcileRunsByStatus        *prometheus.CounterVec
	ReconcileErrorsByType        *prometheus.CounterVec
	ReconcileLastStatus          *prometheus.GaugeVec
	ReconcileLastDurationSeconds prometheus.Gauge
	ReconcileLastMismatchCount   prometheus.Gauge
}

func newMetrics() *metrics {
	return &metrics{
		OrdersSubmitted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_orders_submitted_total",
			Help: "Total live orders submitted",
		}, []string{"strategy"}),

		OrdersFilled: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_orders_filled_total",
			Help: "Total fills received from account streamer",
		}, []string{"strategy"}),

		OrderLatency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:    "tastytrade_order_latency_seconds",
			Help:    "Time from dry-run call to fill event",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		}),

		APIErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_api_errors_total",
			Help: "API errors by HTTP status code",
		}, []string{"status_code"}),

		RateLimitHits: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_rate_limit_hits_total",
			Help: "429 responses received per endpoint family",
		}, []string{"family"}),

		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "tastytrade_request_duration_seconds",
			Help:    "HTTP request duration per family and method",
			Buckets: prometheus.DefBuckets,
		}, []string{"family", "method"}),

		TokenRefreshes: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_token_refresh_total",
			Help: "Token refresh attempts",
		}, []string{"outcome"}), // outcome: ok | fail | missing_refresh_token

		StreamerReconnects: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_streamer_reconnects_total",
			Help: "Reconnect attempts per streamer",
		}, []string{"streamer"}), // streamer: account | market

		StreamerUptime: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "tastytrade_streamer_uptime_seconds",
			Help: "Seconds since last successful streamer connection",
		}, []string{"streamer"}),

		NLQDollars: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_nlq_dollars",
			Help: "Current net liquidating value in USD",
		}),

		OpenPositions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_open_positions",
			Help: "Number of open positions",
		}),

		CircuitBreakerState: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_circuit_breaker_state",
			Help: "0=normal, 1=tripped",
		}),

		KillSwitchState: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_kill_switch_state",
			Help: "0=normal, 1=halted",
		}),

		QuotesReceived: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_quotes_received_total",
			Help: "Total quote events decoded from DXLink FEED_DATA frames",
		}, []string{"symbol"}),

		TrackedSymbols: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_tracked_symbols",
			Help: "Number of symbols currently subscribed on the market streamer",
		}),

		LastQuoteTime: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_last_quote_unix_seconds",
			Help: "Unix timestamp of the most recently received quote event (0 = none yet)",
		}),

		BusDroppedEvents: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_bus_dropped_events_total",
			Help: "Total events dropped by the internal bus because the subscriber channel was full. Non-zero values indicate back-pressure on a consumer; size the channel or speed up the consumer.",
		}, []string{"bus"}),

		ReconcileRunsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tastytrade_reconcile_runs_total",
			Help: "Total reconciliation passes attempted (success + failure).",
		}),
		ReconcileErrorsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tastytrade_reconcile_errors_total",
			Help: "Reconciliation passes that failed due to a REST error. Non-zero values indicate connectivity or auth problems with the Positions endpoint.",
		}),
		ReconcilePositionsCorrected: promauto.NewCounter(prometheus.CounterOpts{
			Name: "tastytrade_reconcile_positions_corrected_total",
			Help: "MarkBook entries patched by the reconciler (new positions added + zero-cost-basis entries corrected).",
		}),
		ReconcileRunsByStatus: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_reconcile_runs_by_status_total",
			Help: "Reconciliation passes by structured outcome status.",
		}, []string{"status"}),
		ReconcileErrorsByType: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "tastytrade_reconcile_errors_by_type_total",
			Help: "Reconciliation errors by coarse error type or error text.",
		}, []string{"type"}),
		ReconcileLastStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "tastytrade_reconcile_last_status",
			Help: "One-hot gauge for the latest reconciliation status by label (1 = latest status, 0 = not latest).",
		}, []string{"status"}),
		ReconcileLastDurationSeconds: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_reconcile_last_duration_seconds",
			Help: "Duration in seconds of the latest reconciliation pass.",
		}),
		ReconcileLastMismatchCount: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "tastytrade_reconcile_last_mismatch_count",
			Help: "Mismatch count from the latest reconciliation pass.",
		}),
	}
}
