// Package client contains the HTTP client, auth flow, and safety controls.
package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KillSwitch returns (true, reason) if order submission must be halted.
// It is called before every order submit and dry-run.
// Always updates the KillSwitchState Prometheus gauge.
//
// Two sources are checked:
//  1. TASTYTRADE_KILL_SWITCH=true env var — fastest, no I/O, survives restarts
//  2. Presence of the kill file in UserConfigDir — can be toggled without env reload:
//     touch ~/.config/tastytrade-cli/KILL   → halt
//     rm    ~/.config/tastytrade-cli/KILL   → resume
func KillSwitch() (bool, string) {
	if strings.ToLower(os.Getenv("TASTYTRADE_KILL_SWITCH")) == "true" {
		Metrics.KillSwitchState.Set(1)
		return true, "TASTYTRADE_KILL_SWITCH env var is set to 'true'"
	}
	kf, err := killFilePath()
	if err == nil {
		if _, err := os.Stat(kf); err == nil {
			Metrics.KillSwitchState.Set(1)
			return true, fmt.Sprintf("kill file present: %s", kf)
		}
	}
	Metrics.KillSwitchState.Set(0)
	return false, ""
}

// KillFilePath returns the path of the file-based kill switch.
// Exported so the gate checklist and startup log can print the path.
func KillFilePath() (string, error) {
	return killFilePath()
}

func killFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine UserConfigDir: %w", err)
	}
	return filepath.Join(dir, "tastytrade-cli", "KILL"), nil
}

// ArmKillSwitch creates the kill file. Idempotent.
func ArmKillSwitch() error {
	kf, err := killFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(kf), 0o700); err != nil {
		return fmt.Errorf("mkdir for kill file: %w", err)
	}
	f, err := os.OpenFile(kf, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create kill file: %w", err)
	}
	return f.Close()
}

// DisarmKillSwitch removes the kill file. Idempotent (no error if absent).
func DisarmKillSwitch() error {
	kf, err := killFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(kf)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove kill file: %w", err)
	}
	return nil
}
