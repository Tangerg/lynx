package turn

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval/approvaltest"
)

// TestApproveToolCall_RememberedShortCircuit verifies the gate consults a
// standing rule BEFORE prompting (B5): a remembered allow passes without an
// interrupt, a remembered deny refuses without one. Both paths avoid
// hitl.Interrupt, so no agent process context is needed.
func TestApproveToolCall_RememberedShortCircuit(t *testing.T) {
	ctx := context.Background()
	appr := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore()) // shell gates → would prompt
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{approval: appr},
		st:         &turnState{handle: TurnHandle{SessionID: "s1"}},
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
// remembered deny still wins) and is scoped to the GatePrompt path (plan-mode's
// GateDeny is never reached, since that's a separate switch case above).
func TestApproveToolCall_MCPAutoApprove(t *testing.T) {
	ctx := context.Background()
	appr := approval.New(approval.ModeSafe, approvaltest.NewMemoryStore()) // unknown tool → exec → would prompt
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{
			approval:            appr,
			mcpToolAutoApproved: func(name string) bool { return name == "srv_read" },
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
	// plumbing and is covered by the TestDispatcher_ApprovalGate_* integration
	// tests, not this bare-construction unit test.)
	_ = appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "srv_read", Arguments: "{}", Decision: approval.Deny,
	})
	if v := obs.ApproveToolCall(ctx, "c2", "srv_read", "{}"); v.Interrupt != nil || !v.Denied {
		t.Fatalf("remembered deny over auto-approve = %+v, want a denied verdict", v)
	}
}
