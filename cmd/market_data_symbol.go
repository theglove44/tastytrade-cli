package cmd

import (
	"regexp"
	"strconv"
	"strings"
)

var occEquityOptionRE = regexp.MustCompile(`^([A-Z\.]{1,6})\s+(\d{6})([CP])(\d{8})$`)

// normalizeMarketDataSymbol converts a raw tastytrade/account position symbol
// into the DXLink streamer-symbol shape when the conversion is obvious and safe.
//
// Source of truth: tastytrade docs require DXLink subscriptions to use
// streamer-symbol values, not raw OCC/account symbols. For equity options,
// streamer symbols are the OCC symbol rewritten as:
//
//	SPY 230731C00393000 -> .SPY230731C393
//
// Non-option symbols are returned unchanged.
func normalizeMarketDataSymbol(symbol, instrumentType string) string {
	trimmed := strings.TrimSpace(symbol)
	if trimmed == "" {
		return trimmed
	}
	if !strings.EqualFold(instrumentType, "Equity Option") {
		return trimmed
	}
	m := occEquityOptionRE.FindStringSubmatch(trimmed)
	if len(m) != 5 {
		return trimmed
	}
	underlying := m[1]
	expiry := m[2]
	right := m[3]
	strikeRaw := m[4]

	strikeInt, err := strconv.Atoi(strikeRaw)
	if err != nil {
		return trimmed
	}
	whole := strikeInt / 1000
	frac := strikeInt % 1000
	strike := strconv.Itoa(whole)
	if frac != 0 {
		fracStr := strconv.Itoa(frac + 1000)[1:]
		fracStr = strings.TrimRight(fracStr, "0")
		strike += "." + fracStr
	}
	return "." + underlying + expiry + right + strike
}
