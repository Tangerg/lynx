package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval/approvaltest"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// TestApproveToolCall_RememberedShortCircuit verifies the gate consults a
// standing rule BEFORE prompting (B5): a remembered allow passes without an
// interrupt, a remembered deny refuses without one. Both paths avoid
// hitl.Interrupt, so no agent process context is needed.
func TestApproveToolCall_RememberedShortCircuit(t *testing.T) {
	ctx := context.Background()
	appr := newTestApprovalPolicy(t, approval.ModeSafe)
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{approval: appr},
		st:         &turnState{handle: TurnHandle{SessionID: "s1"}},
	}

	// Remembered allow → verdict runs (no interrupt, not denied).
	shellArguments := `{"command":"go test"}`
	if err := appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "shell",
		Arguments: mustToolArguments(t, shellArguments), Decision: approval.Allow,
	}); err != nil {
		t.Fatalf("remember shell allow: %v", err)
	}
	if v := obs.ApproveToolCall(ctx, "c1", "shell", shellArguments, agentexec.ToolApprovalTarget{}); v.Interrupt != nil || v.Denied {
		t.Fatalf("remembered allow = %+v, want a clean run verdict", v)
	}

	// Remembered deny → verdict denies (no interrupt).
	writeArguments := `{"file_path":"main.go"}`
	if err := appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: "write",
		Arguments: mustToolArguments(t, writeArguments), Decision: approval.Deny,
	}); err != nil {
		t.Fatalf("remember write deny: %v", err)
	}
	if v := obs.ApproveToolCall(ctx, "c2", "write", writeArguments, agentexec.ToolApprovalTarget{}); v.Interrupt != nil || !v.Denied {
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
	appr := newTestApprovalPolicy(t, approval.ModeSafe)
	autoApproved := mcpserver.ToolRef{Server: "a_b", Tool: "c"}
	colliding := mcpserver.ToolRef{Server: "a", Tool: "b_c"}
	if autoApproved.PublicName() != colliding.PublicName() {
		t.Fatalf("fixture public names do not collide: %q != %q", autoApproved.PublicName(), colliding.PublicName())
	}
	obs := &turnObserver{
		dispatcher: &memoryDispatcher{
			approval:            appr,
			mcpToolAutoApproved: func(ref mcpserver.ToolRef) bool { return ref == autoApproved },
		},
		st: &turnState{handle: TurnHandle{SessionID: "s1"}},
	}

	// Whitelisted MCP tool → passes without an interrupt (no standing rule).
	target := agentexec.ToolApprovalTarget{MCP: autoApproved}
	if v := obs.ApproveToolCall(ctx, "c1", autoApproved.PublicName(), "{}", target); v.Interrupt != nil || v.Denied {
		t.Fatalf("auto-approved tool = %+v, want a clean run verdict", v)
	}
	// The same public name bound to a different wrapper identity must still
	// prompt. This remains true if the live MCP catalog changes after a turn
	// resolved its tools because the identity rides with the actual wrapper.
	if v := obs.ApproveToolCall(ctx, "c-collision", colliding.PublicName(), "{}", agentexec.ToolApprovalTarget{MCP: colliding}); v.Interrupt == nil {
		t.Fatalf("colliding MCP identity unexpectedly auto-approved: %+v", v)
	}

	// A remembered DENY on the same tool wins — the whitelist is consulted only
	// AFTER rules, so an explicit deny is never silently overridden. (A
	// non-whitelisted tool still gates; that prompt path needs real HITL
	// plumbing and is covered by the TestDispatcher_ApprovalGate_* integration
	// tests, not this bare-construction unit test.)
	if err := appr.Remember(ctx, approval.RememberRequest{
		Scope: approval.ScopeSession, SessionID: "s1", Tool: autoApproved.PublicName(),
		Arguments: mustToolArguments(t, `{}`), Decision: approval.Deny,
	}); err != nil {
		t.Fatalf("remember MCP deny: %v", err)
	}
	if v := obs.ApproveToolCall(ctx, "c2", autoApproved.PublicName(), "{}", target); v.Interrupt != nil || !v.Denied {
		t.Fatalf("remembered deny over auto-approve = %+v, want a denied verdict", v)
	}
}

func TestApproveToolCallSurfacesApprovalPolicyFailures(t *testing.T) {
	t.Run("decide", func(t *testing.T) {
		want := errors.New("rule store unavailable")
		observer := approvalObserver(&errorApprovalPolicy{decideErr: want})
		verdict := observer.ApproveToolCall(
			t.Context(), "call_1", "shell", `{"command":"go test"}`, agentexec.ToolApprovalTarget{},
		)
		if !errors.Is(verdict.Interrupt, want) {
			t.Fatalf("approval verdict = %+v, want decision error", verdict)
		}
	})

	t.Run("remember restored response", func(t *testing.T) {
		want := errors.New("rule write unavailable")
		const arguments = `{"command":"go test"}`
		pending := runs.Interrupt{
			Kind: runs.ApprovalInterruptKind,
			Approval: &runs.ApprovalPrompt{
				CallID: "call_1", ToolName: "shell", Arguments: arguments,
				SafetyClass: tool.SafetyClassExec,
			},
		}
		prompt, err := suspension.EncodePrompt(pending)
		if err != nil {
			t.Fatal(err)
		}
		response, err := suspension.EncodeResolution(interrupts.Resolution{
			Approved: true, RememberScope: approval.ScopeSession,
		})
		if err != nil {
			t.Fatal(err)
		}
		key := interrupts.InterruptKey(string(runs.ApprovalInterruptKind), "shell", arguments)
		ctx := core.WithProcessView(t.Context(), suspendedProcessView{value: &interaction.Suspension{
			ID: key, Prompt: prompt, Response: response,
		}})
		verdict := approvalObserver(&errorApprovalPolicy{rememberErr: want}).ApproveToolCall(
			ctx, "call_1", "shell", arguments, agentexec.ToolApprovalTarget{},
		)
		if !errors.Is(verdict.Interrupt, want) {
			t.Fatalf("approval verdict = %+v, want remember error", verdict)
		}
	})
}

func TestApproveToolCallRejectsMalformedGatedArguments(t *testing.T) {
	verdict := approvalObserver(newTestApprovalPolicy(t, approval.ModeSafe)).ApproveToolCall(
		t.Context(), "call_1", "shell", `{"command":`, agentexec.ToolApprovalTarget{},
	)
	if !errors.Is(verdict.Interrupt, tool.ErrInvalidArguments) {
		t.Fatalf("approval verdict = %+v, want invalid tool arguments", verdict)
	}
}

func approvalObserver(policy approval.Policy) *turnObserver {
	return &turnObserver{
		dispatcher: &memoryDispatcher{approval: policy},
		st:         &turnState{handle: TurnHandle{SessionID: "s1"}},
	}
}

type errorApprovalPolicy struct {
	decideErr   error
	rememberErr error
}

func (*errorApprovalPolicy) Mode(context.Context) (approval.Mode, error) {
	return approval.ModeSafe, nil
}

func (*errorApprovalPolicy) SetMode(context.Context, approval.Mode) error { return nil }

func (p *errorApprovalPolicy) Decide(context.Context, approval.Query) (approval.Decision, bool, error) {
	return "", false, p.decideErr
}

func (p *errorApprovalPolicy) Remember(context.Context, approval.RememberRequest) error {
	return p.rememberErr
}

func (*errorApprovalPolicy) Rules(context.Context, string, string) ([]approval.Rule, error) {
	return nil, nil
}

func (*errorApprovalPolicy) Forget(context.Context, string) error { return nil }

type suspendedProcessView struct {
	core.ProcessView
	value *interaction.Suspension
}

func (p suspendedProcessView) Suspension() *interaction.Suspension { return p.value }

func newTestApprovalPolicy(t *testing.T, mode approval.Mode) approval.Policy {
	t.Helper()
	policy, err := approval.New(mode, approvaltest.NewMemoryStore())
	if err != nil {
		t.Fatalf("new approval policy: %v", err)
	}
	return policy
}

func mustToolArguments(t *testing.T, raw string) tool.Arguments {
	t.Helper()
	arguments, err := tool.ParseArguments(raw)
	if err != nil {
		t.Fatalf("parse tool arguments: %v", err)
	}
	return arguments
}
