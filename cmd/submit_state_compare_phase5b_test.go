package cmd

import (
	"strings"
	"testing"
)

func TestSubmitStateManualReconciliationHelpText(t *testing.T) {
	if !strings.Contains(submitStateCmd.Long, "manual reconciliation workflows") {
		t.Fatalf("submitStateCmd.Long = %q, want manual reconciliation guidance", submitStateCmd.Long)
	}
	if !strings.Contains(submitStateCompareCmd.Long, "manual reconciliation workflow") {
		t.Fatalf("submitStateCompareCmd.Long = %q, want manual reconciliation guidance", submitStateCompareCmd.Long)
	}
}
