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
		{"shell", approval.ModePlan, gateDeny},
		{"some_mcp_tool", approval.ModePlan, gateDeny}, // unknown → exec class

		// ModeSafe prompts on every non-read tool.
		{"write", approval.ModeSafe, gatePrompt},
		{"shell", approval.ModeSafe, gatePrompt},

		// ModeBalanced prompts only on exec; write/network auto-pass.
		{"write", approval.ModeBalanced, gatePass},
		{"edit", approval.ModeBalanced, gatePass},
		{"shell", approval.ModeBalanced, gatePrompt},
		{"unknown_tool", approval.ModeBalanced, gatePrompt}, // unknown → exec class

		// ModeYolo passes everything.
		{"shell", approval.ModeYolo, gatePass},
		{"write", approval.ModeYolo, gatePass},
	}
	for _, c := range cases {
		if got := gateFor(c.tool, c.mode); got != c.want {
			t.Errorf("gateFor(%q, %v) = %d, want %d", c.tool, c.mode, got, c.want)
		}
	}
}

// TestApproveToolCall_RememberedShortCircuit verifies the gate consults a
// standing rule BEFORE prompting (B5): a remembered allow passes without an
// interrupt, a remembered deny refuses without one. Both paths avoid
// hitl.Interrupt, so no agent process context is needed.
func TestApproveToolCall_RememberedShortCircuit(t *testing.T) {
	ctx := context.Background()
	appr := approval.New(approval.ModeSafe, approval.NewMemoryStore()) // shell gates → would prompt
	obs := &turnObserver{
		svc: &inMemory{approval: appr},
		st:  &turnState{handle: TurnHandle{SessionID: "s1"}},
	}

	// Remembered allow → verdict runs (no interrupt, not denied).
	_ = appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "shell", Arguments: "{}", Decision: approval.Allow,
	})
	if v := obs.ApproveToolCall(ctx, "c1", "shell", "{}"); v.Interrupt != nil || v.Denied {
		t.Fatalf("remembered allow = %+v, want a clean run verdict", v)
	}

	// Remembered deny → verdict denies (no interrupt).
	_ = appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "write", Arguments: "{}", Decision: approval.Deny,
	})
	if v := obs.ApproveToolCall(ctx, "c2", "write", "{}"); v.Interrupt != nil || !v.Denied {
		t.Fatalf("remembered deny = %+v, want a denied verdict", v)
	}
}

// TestApproveToolCall_MCPAutoApprove verifies the per-server auto-approve
// whitelist: a listed MCP tool skips the prompt that its (unknown → exec class)
// name would otherwise trigger, but the whitelist sits BELOW standing rules (a
// remembered deny still wins) and is scoped to the gatePrompt path (plan-mode's
// gateDeny is never reached, since that's a separate switch case above).
func TestApproveToolCall_MCPAutoApprove(t *testing.T) {
	ctx := context.Background()
	appr := approval.New(approval.ModeSafe, approval.NewMemoryStore()) // unknown tool → exec → would prompt
	autoApprove := map[string]struct{}{"srv_read": {}}
	obs := &turnObserver{
		svc: &inMemory{
			approval:       appr,
			mcpAutoApprove: func() map[string]struct{} { return autoApprove },
		},
		st: &turnState{handle: TurnHandle{SessionID: "s1"}},
	}

	// Whitelisted MCP tool → passes without an interrupt (no standing rule).
	if v := obs.ApproveToolCall(ctx, "c1", "srv_read", "{}"); v.Interrupt != nil || v.Denied {
		t.Fatalf("auto-approved tool = %+v, want a clean run verdict", v)
	}

	// A remembered DENY on the same tool wins — the whitelist is consulted only
	// AFTER rules, so an explicit deny is never silently overridden. (A
	// non-whitelisted tool still gates; that prompt path needs real HITL
	// plumbing and is covered by the TestService_ApprovalGate_* integration
	// tests, not this bare-construction unit test.)
	_ = appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "srv_read", Arguments: "{}", Decision: approval.Deny,
	})
	if v := obs.ApproveToolCall(ctx, "c2", "srv_read", "{}"); v.Interrupt != nil || !v.Denied {
		t.Fatalf("remembered deny over auto-approve = %+v, want a denied verdict", v)
	}
}
