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

		// ModeSafe prompts on every non-read tool.
		{tool.SafetyClassWrite, ModeSafe, GatePrompt},
		{tool.SafetyClassExec, ModeSafe, GatePrompt},

		// ModeBalanced prompts only on exec; write auto-passes.
		{tool.SafetyClassWrite, ModeBalanced, GatePass},
		{tool.SafetyClassExec, ModeBalanced, GatePrompt},

		// ModeYolo passes everything.
		{tool.SafetyClassExec, ModeYolo, GatePass},
		{tool.SafetyClassWrite, ModeYolo, GatePass},
	}
	for _, c := range cases {
		if got := GateFor(c.cls, c.mode); got != c.want {
			t.Errorf("GateFor(%v, %v) = %d, want %d", c.cls, c.mode, got, c.want)
		}
	}
}
