package agentexec

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
	toolcontract "github.com/Tangerg/lynx/tool"
)

// InteractionApprovalPolicy is the exact product policy view required at the
// Interaction Tool boundary.
type InteractionApprovalPolicy interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	Decide(ctx context.Context, query approval.Query) (approval.Decision, bool, error)
	Remember(ctx context.Context, request approval.RememberRequest) error
}

// ToolAuthorizer evaluates Runtime approval policy independently of Agent Framework.
// It returns a durable product prompt when a person is required; the executor
// ACL alone maps that prompt to an Interaction wait and response Signal.
type ToolAuthorizer struct{ policy InteractionApprovalPolicy }

// NewToolAuthorizer binds the product approval policy.
func NewToolAuthorizer(policy InteractionApprovalPolicy) (*ToolAuthorizer, error) {
	if policy == nil || isNilInteractionCapability(policy) {
		return nil, errors.New("agentexec: Tool approval policy is required")
	}
	return &ToolAuthorizer{policy: policy}, nil
}

func (authorizer *ToolAuthorizer) AuthorizeTool(
	ctx context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	if authorizer == nil || authorizer.policy == nil {
		return ToolAuthorizationDecision{}, errors.New("agentexec: Tool authorizer is unavailable")
	}
	if err := validateToolAuthorizationRequest(request); err != nil {
		return ToolAuthorizationDecision{}, err
	}
	mode, err := authorizer.policy.Mode(ctx, request.SessionID)
	if err != nil {
		return ToolAuthorizationDecision{}, fmt.Errorf("agentexec: read Tool approval mode: %w", err)
	}
	plan := (approval.ToolCallInput{
		Arguments:    request.Arguments.Canonical(),
		Mode:         mode,
		Hook:         approval.HookDecision{Ask: request.RequireApproval},
		SafetyClass:  request.SafetyClass,
		FileMutation: request.FileMutation,
		ShellCommand: request.ShellCommand,
	}).Plan()
	if plan.Action == approval.GatePrompt {
		decision, matched, err := authorizer.policy.Decide(ctx, approval.Query{
			SessionID:  request.SessionID,
			ProjectDir: request.CWD,
			Tool:       request.ToolName,
			Subject:    request.ApprovalSubject,
		})
		if err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("agentexec: evaluate remembered Tool approval: %w", err)
		}
		plan = plan.ResolvePromptShortcuts(
			approval.StandingDecision{Decision: decision, Matched: matched},
			request.AutoApproved,
		)
	}
	switch plan.Action {
	case approval.GatePass:
		return ToolAuthorizationDecision{}, nil
	case approval.GateDeny:
		return ToolAuthorizationDecision{
			Denied: true,
			Reason: approvalDenialMessage(plan.Denial, request.ToolName),
		}, nil
	case approval.GatePrompt:
		prompt := runs.ApprovalPrompt{
			CallID:       request.CallID,
			ToolName:     request.ToolName,
			Arguments:    plan.Arguments,
			SafetyClass:  plan.SafetyClass,
			Risk:         plan.Risk,
			Reason:       approvalPromptReason(plan.PromptCause),
			Rememberable: true,
		}
		return ToolAuthorizationDecision{Approval: &prompt}, nil
	default:
		return ToolAuthorizationDecision{}, errors.New("agentexec: Tool approval policy returned an unknown action")
	}
}

func (authorizer *ToolAuthorizer) ResolveToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (ToolAuthorizationDecision, error) {
	if authorizer == nil || authorizer.policy == nil {
		return ToolAuthorizationDecision{}, errors.New("agentexec: Tool authorizer is unavailable")
	}
	if err := validateToolAuthorizationRequest(request); err != nil {
		return ToolAuthorizationDecision{}, err
	}
	if err := (runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt}).Validate(); err != nil {
		return ToolAuthorizationDecision{}, err
	}
	if prompt.CallID != request.CallID || prompt.ToolName != request.ToolName ||
		prompt.Arguments != request.Arguments.Canonical() || prompt.SafetyClass != request.SafetyClass {
		return ToolAuthorizationDecision{}, errors.New("agentexec: Tool approval response differs from its invocation")
	}
	if prompt.Rememberable && resolution.RememberScope != "" {
		if err := authorizer.policy.Remember(ctx, approval.RememberRequest{
			Scope:      resolution.RememberScope,
			SessionID:  request.SessionID,
			ProjectDir: request.CWD,
			Tool:       request.ToolName,
			Subject:    request.ApprovalSubject,
			Decision:   approval.DecisionOf(resolution.Approved),
		}); err != nil {
			return ToolAuthorizationDecision{}, fmt.Errorf("agentexec: remember Tool approval: %w", err)
		}
	}
	if !resolution.Approved {
		return ToolAuthorizationDecision{Denied: true, Reason: denialReason(resolution.Reason)}, nil
	}
	effective := cmp.Or(resolution.Arguments, request.Arguments.Canonical())
	arguments, err := tool.ParseArguments(effective)
	if err != nil {
		return ToolAuthorizationDecision{}, fmt.Errorf("agentexec: parse approved Tool arguments: %w", err)
	}
	if arguments.Canonical() == request.Arguments.Canonical() {
		return ToolAuthorizationDecision{}, nil
	}
	return ToolAuthorizationDecision{EffectiveArguments: &arguments}, nil
}

func validateToolAuthorizationRequest(request ToolAuthorizationRequest) error {
	for name, value := range map[string]string{
		"SessionID": request.SessionID,
		"CWD":       request.CWD,
		"CallID":    request.CallID,
		"ToolName":  request.ToolName,
	} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("agentexec: Tool authorization %s is required without surrounding whitespace", name)
		}
	}
	if !request.SafetyClass.Valid() {
		return fmt.Errorf("agentexec: Tool authorization has invalid safety class %q", request.SafetyClass)
	}
	if request.Arguments.Canonical() == "" {
		return errors.New("agentexec: Tool authorization arguments are required")
	}
	return nil
}

type interactionMCPToolIdentity interface {
	MCPToolIdentity() (sourceName, remoteName string)
}

type interactionFileMutationReporter interface {
	MutationPaths(arguments string) ([]string, error)
}

func fileMutationScope(
	executable toolcontract.Tool,
	arguments tool.Arguments,
	cwd string,
) tool.FileMutationScope {
	reporter, found, err := toolcontract.Capability[interactionFileMutationReporter](executable)
	if err != nil || !found || strings.TrimSpace(cwd) == "" {
		return tool.FileMutationNone
	}
	paths, err := reporter.MutationPaths(arguments.Canonical())
	if err != nil {
		return tool.FileMutationUnknown
	}
	if len(paths) == 0 {
		return tool.FileMutationNone
	}
	root, err := pathidentity.Resolve("", cwd)
	if err != nil {
		return tool.FileMutationUnknown
	}
	for _, path := range paths {
		target, err := pathidentity.Resolve(root, path)
		if err != nil {
			return tool.FileMutationUnknown
		}
		inside, err := pathidentity.Contains(root, target)
		if err != nil {
			return tool.FileMutationUnknown
		}
		if !inside {
			return tool.FileMutationOutsideWorkspace
		}
	}
	return tool.FileMutationWithinWorkspace
}

func approvalDenialMessage(denial approval.Denial, toolName string) string {
	switch denial.Cause {
	case approval.DenialHook:
		if denial.Detail != "" {
			return denial.Detail
		}
		return "denied by a PreToolUse hook"
	case approval.DenialPlanMode:
		return fmt.Sprintf("plan mode is active (read-only): %s is not permitted. Continue investigating with read-only tools or request Plan approval before making changes.", toolName)
	case approval.DenialRememberedRule:
		return "tool call denied by a remembered rule"
	default:
		return "tool call denied by approval policy"
	}
}

func approvalPromptReason(cause approval.PromptCause) string {
	switch cause {
	case approval.PromptCauseRead:
		return "Reads data without changing the workspace."
	case approval.PromptCauseWorkspaceWrite:
		return "Modifies files in the workspace."
	case approval.PromptCauseWorkspaceCommand:
		return "Runs commands in the workspace."
	case approval.PromptCauseNetworkAccess:
		return "Accesses network resources."
	case approval.PromptCauseOutsideWorkspace:
		return "Targets a path outside the workspace directory."
	case approval.PromptCauseUnknownMutation:
		return "Has filesystem mutation targets that could not be verified."
	case approval.PromptCauseCatastrophicCommand:
		return "Runs a high-confidence catastrophic shell command."
	default:
		return "Has an unknown safety classification."
	}
}

func denialReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "tool call denied by user"
	}
	return strings.TrimSpace(reason)
}

var _ InteractionToolAuthorizer = (*ToolAuthorizer)(nil)
