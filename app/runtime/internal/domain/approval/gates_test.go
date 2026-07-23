package approval

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// TestGateFor_Matrix audits the full (tool-class × mode) → action matrix.
// GateFor is a pure function, so the table is the spec.
func TestGateFor_Matrix(t *testing.T) {
	cases := []struct {
		cls  tool.SafetyClass
		mode Mode
		want GateAction
	}{
		// Read-only tools never gate, in any mode.
		{tool.SafetyClassSafe, ModePlan, GatePass},
		{tool.SafetyClassSafe, ModeSafe, GatePass},
		{tool.SafetyClassSafe, ModeBalanced, GatePass},
		{tool.SafetyClassSafe, ModeYolo, GatePass},

		// ModePlan denies every non-read tool outright (read-only).
		{tool.SafetyClassWrite, ModePlan, GateDeny},
		{tool.SafetyClassExec, ModePlan, GateDeny},
		{tool.SafetyClassNetwork, ModePlan, GateDeny},
		{"", ModePlan, GateDeny},

		// ModeSafe prompts on every non-read tool.
		{tool.SafetyClassWrite, ModeSafe, GatePrompt},
		{tool.SafetyClassExec, ModeSafe, GatePrompt},
		{tool.SafetyClassNetwork, ModeSafe, GatePrompt},
		{"", ModeSafe, GatePrompt},

		// ModeBalanced auto-passes known write/network calls; exec and unknown
		// classes prompt.
		{tool.SafetyClassWrite, ModeBalanced, GatePass},
		{tool.SafetyClassExec, ModeBalanced, GatePrompt},
		{tool.SafetyClassNetwork, ModeBalanced, GatePass},
		{"", ModeBalanced, GatePrompt},

		// ModeYolo passes everything.
		{tool.SafetyClassExec, ModeYolo, GatePass},
		{tool.SafetyClassWrite, ModeYolo, GatePass},
		{"", ModeYolo, GatePass},

		// Unknown modes fail closed for side-effecting and unknown classes.
		{tool.SafetyClassSafe, Mode(99), GatePass},
		{tool.SafetyClassExec, Mode(99), GatePrompt},
		{"", Mode(99), GatePrompt},
	}
	for _, c := range cases {
		if got := GateFor(c.cls, c.mode); got != c.want {
			t.Errorf("GateFor(%v, %v) = %d, want %d", c.cls, c.mode, got, c.want)
		}
	}
}
