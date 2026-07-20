package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestStuckDecision(t *testing.T) {
	tests := []struct {
		decision core.StuckDecision
		valid    bool
		text     string
	}{
		{decision: core.StuckStop, valid: true, text: "stop"},
		{decision: core.StuckReplan, valid: true, text: "replan"},
		{decision: core.StuckDecision(9), text: "StuckDecision(9)"},
	}
	for _, test := range tests {
		if got := test.decision.Valid(); got != test.valid {
			t.Errorf("%d.Valid() = %t, want %t", test.decision, got, test.valid)
		}
		if got := test.decision.String(); got != test.text {
			t.Errorf("%d.String() = %q, want %q", test.decision, got, test.text)
		}
	}
}
