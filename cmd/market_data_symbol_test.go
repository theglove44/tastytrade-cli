package cmd

import "testing"

func TestNormalizeMarketDataSymbol_EquityOption_OCCToStreamer(t *testing.T) {
	got := normalizeMarketDataSymbol("SPY   260417P00650000", "Equity Option")
	want := ".SPY260417P650"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeMarketDataSymbol_EquityOption_WithFractionalStrike(t *testing.T) {
	got := normalizeMarketDataSymbol("SPY 230731C00393500", "Equity Option")
	want := ".SPY230731C393.5"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeMarketDataSymbol_EquityPassThrough(t *testing.T) {
	got := normalizeMarketDataSymbol("SPY", "Equity")
	if got != "SPY" {
		t.Fatalf("got %q, want SPY", got)
	}
}

func TestNormalizeMarketDataSymbol_UnrecognizedPassThrough(t *testing.T) {
	raw := "./E3AN23P5600:XCME"
	got := normalizeMarketDataSymbol(raw, "Future Option")
	if got != raw {
		t.Fatalf("got %q, want %q", got, raw)
	}
}
