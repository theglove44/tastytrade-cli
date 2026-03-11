// Package metrics exposes a Prometheus /metrics HTTP endpoint.
//
// The server binds to 127.0.0.1 only and must never be exposed on 0.0.0.0.
// Default address: 127.0.0.1:9090
// Override:        TASTYTRADE_METRICS_ADDR=127.0.0.1:9091
package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const defaultAddr = "127.0.0.1:9090"

// Addr returns the configured bind address, defaulting to 127.0.0.1:9090.
// It enforces the localhost-only invariant: if a non-loopback address is
// configured the default is returned and a warning is logged.
func Addr(log *zap.Logger) string {
	addr := os.Getenv("TASTYTRADE_METRICS_ADDR")
	if addr == "" {
		return defaultAddr
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Warn("TASTYTRADE_METRICS_ADDR is malformed — using default",
			zap.String("configured", addr),
			zap.String("default", defaultAddr),
			zap.Error(err),
		)
		return defaultAddr
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		log.Warn("TASTYTRADE_METRICS_ADDR must be a loopback address — using default",
			zap.String("configured", addr),
			zap.String("default", defaultAddr),
		)
		return defaultAddr
	}
	return addr
}

// Serve starts the metrics HTTP server in a background goroutine.
// It returns immediately; the server runs until ctx is cancelled.
//
// The /metrics endpoint is served by promhttp.Handler() which uses the
// default Prometheus registry populated by promauto throughout the codebase.
//
// Only /metrics is registered — no other routes.
func Serve(ctx context.Context, addr string, log *zap.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Info("metrics server starting", zap.String("addr", "http://"+addr+"/metrics"))

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server error", zap.Error(err))
		}
	}()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Warn("metrics server shutdown error", zap.Error(err))
		}
	}()
}
