package turn

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// TestGateFor_Matrix audits the full (tool-class × mode) → action
// matrix. gateFor is a pure function, so the table is the spec.
func TestGateFor_Matrix(t *testing.T) {
	cases := []struct {
		tool string
		mode approval.Mode
		want gateAction
	}{
		// Read-only tools never gate, in any mode.
		{"read", approval.ModePlan, gatePass},
		{"grep", approval.ModeSafe, gatePass},
		{"glob", approval.ModeBalanced, gatePass},
		{"read", approval.ModeYolo, gatePass},

		// ModePlan denies every non-read tool outright (read-only).
		{"write", approval.ModePlan, gateDeny},
		{"edit", approval.ModePlan, gateDeny},
		{"bash", approval.ModePlan, gateDeny},
		{"some_mcp_tool", approval.ModePlan, gateDeny}, // unknown → exec class

		// ModeSafe prompts on every non-read tool.
		{"write", approval.ModeSafe, gatePrompt},
		{"bash", approval.ModeSafe, gatePrompt},

		// ModeBalanced prompts only on exec; write/network auto-pass.
		{"write", approval.ModeBalanced, gatePass},
		{"edit", approval.ModeBalanced, gatePass},
		{"bash", approval.ModeBalanced, gatePrompt},
		{"unknown_tool", approval.ModeBalanced, gatePrompt}, // unknown → exec class

		// ModeYolo passes everything.
		{"bash", approval.ModeYolo, gatePass},
		{"write", approval.ModeYolo, gatePass},
	}
	for _, c := range cases {
		if got := gateFor(c.tool, c.mode); got != c.want {
			t.Errorf("gateFor(%q, %v) = %d, want %d", c.tool, c.mode, got, c.want)
		}
	}
}

// TestApproveToolCall_RememberedShortCircuit verifies the gate consults a
// standing session decision BEFORE prompting (B5): a remembered approve passes
// without an interrupt, a remembered deny refuses without one. Both paths
// avoid hitl.Interrupt, so no agent process context is needed.
func TestApproveToolCall_RememberedShortCircuit(t *testing.T) {
	ctx := context.Background()
	appr := approval.New(approval.ModeSafe) // bash gates → would prompt
	obs := &turnObserver{
		svc: &inMemory{approval: appr},
		st:  &turnState{handle: TurnHandle{SessionID: "s1"}},
	}

	// Remembered approve → verdict runs (no interrupt, not denied).
	_ = appr.Remember(ctx, "s1", "bash", true)
	if v := obs.ApproveToolCall(ctx, "c1", "bash", "{}"); v.Interrupt != nil || v.Denied {
		t.Fatalf("remembered approve = %+v, want a clean run verdict", v)
	}

	// Remembered deny → verdict denies (no interrupt).
	_ = appr.Remember(ctx, "s1", "write", false)
	if v := obs.ApproveToolCall(ctx, "c2", "write", "{}"); v.Interrupt != nil || !v.Denied {
		t.Fatalf("remembered deny = %+v, want a denied verdict", v)
	}
}
