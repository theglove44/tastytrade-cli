package streamer

import (
	"math"
	"time"
)

// BackoffPolicy defines the reconnect wait schedule shared by all streamers.
// The backoff is purely exponential: wait = Initial * Factor^failures, capped at Max.
type BackoffPolicy struct {
	Initial time.Duration // base wait duration
	Max     time.Duration // ceiling — never exceeded
	Factor  float64       // multiplier per consecutive failure
}

// DefaultBackoff is the spec-mandated schedule.
// 2s → 4s → 8s → 16s → 32s → 60s (cap).
var DefaultBackoff = BackoffPolicy{
	Initial: 2 * time.Second,
	Max:     60 * time.Second,
	Factor:  2.0,
}

// Next returns the wait duration for the given consecutive failure count (0-based).
// failures=0 returns Initial. Result is always capped at Max.
func (p BackoffPolicy) Next(failures int) time.Duration {
	if failures <= 0 {
		return p.Initial
	}
	d := float64(p.Initial) * math.Pow(p.Factor, float64(failures))
	if d > float64(p.Max) {
		return p.Max
	}
	return time.Duration(d)
}
