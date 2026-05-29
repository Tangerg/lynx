package chat

import (
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
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
		{"read", approval.ModeReadOnly, gatePass},
		{"grep", approval.ModeSafe, gatePass},
		{"glob", approval.ModeBalanced, gatePass},
		{"read", approval.ModeYolo, gatePass},

		// ModeReadOnly denies every non-read tool outright.
		{"write", approval.ModeReadOnly, gateDeny},
		{"edit", approval.ModeReadOnly, gateDeny},
		{"bash", approval.ModeReadOnly, gateDeny},
		{"some_mcp_tool", approval.ModeReadOnly, gateDeny}, // unknown → exec class

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
