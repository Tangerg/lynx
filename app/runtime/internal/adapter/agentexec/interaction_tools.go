package agentexec

import (
	"context"
	"errors"
	"fmt"
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
	Manifest(ctx context.Context, group tool.Group) (toolset.Manifest, error)
}

// InteractionToolInterpreter owns product policy facts implied by concrete
// Tool identities. Toolset satisfies this port without importing Agent Framework. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolInterpreter interface {
	SafetyClass(name string) tool.SafetyClass
	UsesStandardPolicy(name string) bool
	ApprovalSubject(name string, arguments tool.Arguments) (string, error)
	ShellCommand(name, arguments string) string
	ProjectOutcome(ctx context.Context, sessionID, name string, succeeded bool) (runs.ExecutionFact, error)
}

// InteractionToolPresenter owns client-facing activity and result projection.
// Its implementation remains in Toolset; Agent Framework sees only ordinary Tools. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolPresenter interface {
	Activity(name string, arguments tool.Arguments) string
	Present(name string, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

// ToolAuthorizationRequest is the complete pre-call policy input. The
// authorizer may allow, rewrite, deny, or require durable human approval.
type ToolAuthorizationRequest struct {
	SessionID       string
	CWD             string
	CallID          string
	ToolName        string
	Arguments       tool.Arguments
	SafetyClass     tool.SafetyClass
	ApprovalSubject string
	FileMutation    tool.FileMutationScope
	ShellCommand    string
	AutoApproved    bool
	RequireApproval bool
}

// ToolAuthorizationDecision is one definite pre-call decision. Denied calls return
// Reason to the model as a recoverable Tool result. EffectiveArguments is nil
// to preserve the model arguments or non-nil to replace them atomically before
// the ToolCallStarted fact is committed.
type ToolAuthorizationDecision struct {
	Denied             bool
	Reason             string
	EffectiveArguments *tool.Arguments
	Approval           *runs.ApprovalPrompt
}

// InteractionToolAuthorizer evaluates Runtime Tool policy and resolves its
// optional durable human response. It plans but never owns the wait lifecycle:
// interactioninput remains the sole Agent Framework ACL. Implementations must be safe
// for concurrent calls from one Tool batch.
type InteractionToolAuthorizer interface {
	AuthorizeTool(ctx context.Context, request ToolAuthorizationRequest) (ToolAuthorizationDecision, error)
	ResolveToolApproval(
		ctx context.Context,
		request ToolAuthorizationRequest,
		prompt runs.ApprovalPrompt,
		resolution interrupt.Resolution,
	) (ToolAuthorizationDecision, error)
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
	RequireApproval    bool
}

// InteractionToolHooks owns Runtime lifecycle extensions around ordinary Tool
// calls. PostToolUse is observational: its error is recorded by the caller but
// cannot rewrite a Tool result after the external operation completed. An
// implementation must be safe for concurrent calls from one Tool batch.
type InteractionToolHooks interface {
	BeforeToolUse(ctx context.Context, input InteractionToolHookInput) (InteractionToolHookDecision, error)
	AfterToolUse(ctx context.Context, input InteractionToolHookInput) error
}

func validateToolAuthorizationDecision(decision ToolAuthorizationDecision) error {
	if decision.Denied && (decision.EffectiveArguments != nil || decision.Approval != nil) {
		return errors.New("agentexec: denied Tool authorization carries an argument rewrite or approval")
	}
	if decision.Approval != nil && decision.EffectiveArguments != nil {
		return errors.New("agentexec: pending Tool approval rewrites arguments outside its prompt")
	}
	if decision.Reason != strings.TrimSpace(decision.Reason) {
		return errors.New("agentexec: Tool authorization decision reason has surrounding whitespace")
	}
	if !decision.Denied && decision.Reason != "" {
		return errors.New("agentexec: allowed Tool authorization decision carries a denial reason")
	}
	if decision.Denied && decision.Reason == "" {
		return errors.New("agentexec: denied Tool authorization decision requires a reason")
	}
	if decision.Approval != nil {
		if err := (runs.Interrupt{Kind: interrupt.Approval, Approval: decision.Approval}).Validate(); err != nil {
			return fmt.Errorf("agentexec: invalid Tool approval plan: %w", err)
		}
	}
	return nil
}

func validateHookDecision(decision InteractionToolHookDecision) error {
	if decision.Denied && (decision.EffectiveArguments != nil || decision.RequireApproval) {
		return errors.New("agentexec: denied Tool hook decision rewrites arguments or requests approval")
	}
	if decision.Reason != strings.TrimSpace(decision.Reason) {
		return errors.New("agentexec: Tool hook decision reason has surrounding whitespace")
	}
	if !decision.Denied && !decision.RequireApproval && decision.Reason != "" {
		return errors.New("agentexec: allowed Tool hook decision carries a denial reason")
	}
	if decision.Denied && decision.Reason == "" {
		return errors.New("agentexec: denied Tool hook decision requires a reason")
	}
	return nil
}
