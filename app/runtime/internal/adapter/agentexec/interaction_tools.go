package agentexec

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// InteractionToolResolver builds the exact Tool manifest for one staged root.
// Runtime binds the resolved Session/workspace scope to ctx before calling it;
// resolution must not execute a Tool or call a model. [toolset.Resolver]
// satisfies this port directly.
type InteractionToolResolver interface {
	Manifest(ctx context.Context, role string) (toolset.Manifest, error)
}

// InteractionToolInterpreter owns product policy facts implied by concrete
// Tool identities. Toolset satisfies this port without importing Agent2. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolInterpreter interface {
	SafetyClass(name string) tool.SafetyClass
	UsesStandardPolicy(name string) bool
	ProjectOutcome(ctx context.Context, sessionID, name string, succeeded bool) (runs.ExecutionFact, error)
}

// InteractionToolPresenter owns client-facing activity and result projection.
// Its implementation remains in Toolset; Agent2 sees only ordinary Tools. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolPresenter interface {
	Activity(name string, arguments tool.Arguments) string
	Present(name string, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

// AutomaticToolRequest is the complete non-interactive approval input. An
// automatic gate may only allow, rewrite, or deny this invocation; requesting
// human input belongs to the separate waiting capability.
type AutomaticToolRequest struct {
	SessionID   string
	CWD         string
	CallID      string
	ToolName    string
	Arguments   tool.Arguments
	SafetyClass tool.SafetyClass
}

// AutomaticToolDecision is one definite pre-call decision. Denied calls return
// Reason to the model as a recoverable Tool result. EffectiveArguments is nil
// to preserve the model arguments or non-nil to replace them atomically before
// the ToolCallStarted fact is committed.
type AutomaticToolDecision struct {
	Denied             bool
	Reason             string
	EffectiveArguments *tool.Arguments
}

// AutomaticToolAuthorizer evaluates ordinary calls without user interaction.
// It must return in bounded time, cannot suspend execution, and must be safe for
// concurrent calls from one Tool batch.
type AutomaticToolAuthorizer interface {
	AuthorizeTool(ctx context.Context, request AutomaticToolRequest) (AutomaticToolDecision, error)
}

// InteractiveToolAuthorizer owns the human-approval portion of standard Tool
// policy. PlanToolApproval returns found=false when the already-automatic
// decision needs no person; ResolveToolApproval applies the durable response,
// including remembered-rule side effects, without rerunning the original plan.
type InteractiveToolAuthorizer interface {
	PlanToolApproval(
		ctx context.Context,
		request AutomaticToolRequest,
	) (prompt runs.ApprovalPrompt, found bool, err error)
	ResolveToolApproval(
		ctx context.Context,
		request AutomaticToolRequest,
		prompt runs.ApprovalPrompt,
		resolution interrupt.Resolution,
	) (AutomaticToolDecision, error)
}

// InteractionToolHookInput identifies one ordinary Tool lifecycle callback.
// Result and CallError are present only after execution.
type InteractionToolHookInput struct {
	SessionID string
	CWD       string
	CallID    string
	ToolName  string
	Arguments tool.Arguments
	Result    string
	CallError error
}

// InteractionToolHookDecision is the pre-call hook result. A hook may rewrite
// arguments or deny the invocation but cannot request human input.
type InteractionToolHookDecision struct {
	Denied             bool
	Reason             string
	EffectiveArguments *tool.Arguments
}

// InteractionToolHooks owns Runtime lifecycle extensions around ordinary Tool
// calls. PostToolUse is observational: its error is recorded by the caller but
// cannot rewrite a Tool result after the external operation completed. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolHooks interface {
	BeforeToolUse(ctx context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error)
	AfterToolUse(ctx context.Context, input InteractionToolHookInput) error
}

func validateAutomaticDecision(decision AutomaticToolDecision) error {
	if decision.Denied && decision.EffectiveArguments != nil {
		return errors.New("agentexec: denied automatic Tool decision rewrites arguments")
	}
	if decision.Reason != strings.TrimSpace(decision.Reason) {
		return errors.New("agentexec: automatic Tool decision reason has surrounding whitespace")
	}
	if !decision.Denied && decision.Reason != "" {
		return errors.New("agentexec: allowed automatic Tool decision carries a denial reason")
	}
	if decision.Denied && decision.Reason == "" {
		return errors.New("agentexec: denied automatic Tool decision requires a reason")
	}
	return nil
}

func validateHookDecision(decision InteractionToolHookDecision) error {
	if decision.Denied && decision.EffectiveArguments != nil {
		return errors.New("agentexec: denied Tool hook decision rewrites arguments")
	}
	if decision.Reason != strings.TrimSpace(decision.Reason) {
		return errors.New("agentexec: Tool hook decision reason has surrounding whitespace")
	}
	if !decision.Denied && decision.Reason != "" {
		return errors.New("agentexec: allowed Tool hook decision carries a denial reason")
	}
	if decision.Denied && decision.Reason == "" {
		return errors.New("agentexec: denied Tool hook decision requires a reason")
	}
	return nil
}
