// Package intentlog writes append-only order intent records before any order
// POST is executed.
//
// Purpose: forensic reconstruction if execution behaviour ever diverges from
// what the automation pipeline intended to submit.
//
// Design principles:
//   - Written BEFORE the HTTP request is dispatched (not after)
//   - Append-only: entries are never modified or deleted
//   - Non-blocking: write errors are logged but do not halt execution
//   - One JSON record per line (NDJSON), each ending with \n
//   - File location: os.UserConfigDir()/tastytrade-cli/orders_intent.log
//
// Fields per record (all strings for portability):
//
//	timestamp        RFC3339 UTC
//	account_id       TastyTrade account number
//	symbol           primary symbol (first leg, or empty for complex orders)
//	strategy         caller-provided label (e.g. "iron_condor", "credit_spread")
//	quantity         formatted decimal quantity of first leg
//	price            limit price as string
//	price_effect     "Debit" or "Credit"
//	order_type       e.g. "Limit", "Market"
//	time_in_force    e.g. "Day", "GTC"
//	leg_count        number of legs
//	request_id       X-Request-ID from client middleware
//	idempotency_key  X-Idempotency-Key from client middleware
package intentlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Entry is a single order intent record written to the log file.
// All fields are strings so the log is human-readable and grep-friendly.
type Entry struct {
	Timestamp      string `json:"timestamp"`
	AccountID      string `json:"account_id"`
	Symbol         string `json:"symbol"`
	Strategy       string `json:"strategy"`
	Quantity       string `json:"quantity"`
	Price          string `json:"price"`
	PriceEffect    string `json:"price_effect"`
	OrderType      string `json:"order_type"`
	TimeInForce    string `json:"time_in_force"`
	LegCount       int    `json:"leg_count"`
	RequestID      string `json:"request_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// logPath returns the path to the intent log file.
func logPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("intentlog: cannot determine UserConfigDir: %w", err)
	}
	return filepath.Join(dir, "tastytrade-cli", "orders_intent.log"), nil
}

// Write appends entry to the intent log file as a single JSON line.
// It is a best-effort write: if the file cannot be opened or written,
// the error is logged via zap but execution is NOT halted.
// Always call Write BEFORE dispatching the HTTP order request.
func Write(e Entry, log *zap.Logger) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	path, err := logPath()
	if err != nil {
		log.Warn("intentlog: cannot determine log path",
			zap.Error(err),
			zap.String("idempotency_key", e.IdempotencyKey),
		)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		log.Warn("intentlog: cannot create log directory",
			zap.String("path", path),
			zap.Error(err),
			zap.String("idempotency_key", e.IdempotencyKey),
		)
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Warn("intentlog: cannot open log file",
			zap.String("path", path),
			zap.Error(err),
			zap.String("idempotency_key", e.IdempotencyKey),
		)
		return
	}
	defer f.Close()

	data, err := json.Marshal(e)
	if err != nil {
		log.Warn("intentlog: marshal failed",
			zap.Error(err),
			zap.String("idempotency_key", e.IdempotencyKey),
		)
		return
	}

	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		log.Warn("intentlog: write failed",
			zap.String("path", path),
			zap.Error(err),
			zap.String("idempotency_key", e.IdempotencyKey),
		)
	}
}

// LogPath returns the resolved path of the intent log file for display purposes.
func LogPath() (string, error) {
	return logPath()
}
