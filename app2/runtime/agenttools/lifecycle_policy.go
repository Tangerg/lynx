package agenttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/domain/approvalpolicy"
	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const lifecycleApprovalStateKind = "lyra.lifecycle.approval/v1"

type lifecyclePolicyTool struct {
	toolcontract.Tool
	hooks          LifecycleHooks
	policy         ApprovalPolicy
	scope          agentexec.ToolScope
	safety         protocol.SafetyClass
	paths          mutationPaths
	autoApproved   bool
	intrinsicInput bool
}

func (tool *lifecyclePolicyTool) Unwrap() toolcontract.Tool { return tool.Tool }

type lifecycleApprovalState struct {
	Kind         string `json:"kind"`
	Arguments    string `json:"arguments"`
	Subject      string `json:"subject,omitempty"`
	Rememberable bool   `json:"rememberable,omitempty"`
}

type lifecycleApprovalResponse struct {
	Decision   protocol.ApprovalDecision `json:"decision"`
	Remember   *protocol.RememberScope   `json:"remember,omitempty"`
	EditedArgs map[string]any            `json:"editedArgs,omitempty"`
	Reason     string                    `json:"reason,omitempty"`
}

var lifecycleApprovalSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"decision":{"type":"string","enum":["approve","deny"]},"remember":{"type":"object","additionalProperties":false,"properties":{"scope":{"type":"string","enum":["session","project","global"]}},"required":["scope"]},"editedArgs":{"type":"object"},"reason":{"type":"string"}},"required":["decision"]}`)

func (tool *lifecyclePolicyTool) Call(
	ctx context.Context,
	arguments string,
) (string, error) {
	invocation, ok := agentexec.ToolInvocationFromContext(ctx)
	if !ok {
		return "", errors.New("agenttools: lifecycle policy called outside an Interaction")
	}
	if continuation, resumed := agentexec.ToolInputContinuationFromContext(ctx); resumed {
		var state lifecycleApprovalState
		if err := json.Unmarshal(continuation.State(), &state); err == nil &&
			state.Kind == lifecycleApprovalStateKind {
			return tool.resumeApproval(ctx, invocation, continuation, state)
		}
		if !tool.intrinsicInput {
			return "", errors.New("agenttools: invalid lifecycle approval continuation")
		}
		return tool.callAndEnrich(ctx, invocation, arguments, "")
	}
	decision, err := tool.before(ctx, invocation, arguments)
	if err != nil {
		return "", err
	}
	if decision.Denied() {
		return "", fmt.Errorf("tool denied by lifecycle policy: %s", denialReason(decision))
	}
	effective := arguments
	if decision.RewriteArguments != "" {
		effective = decision.RewriteArguments
	}
	policyPrompt := false
	if !tool.intrinsicInput && tool.safety != protocol.SafetyClassSafe {
		mode, err := tool.policy.Mode(ctx)
		if err != nil {
			return "", fmt.Errorf("agenttools: read approval mode: %w", err)
		}
		policyPrompt = mode.RequiresApproval(effectFromSafety(tool.safety))
	}
	hookPrompt := decision.Asks()
	subject, err := tool.approvalSubject(effective)
	if err != nil {
		return "", err
	}
	catastrophic := !tool.intrinsicInput && tool.safety == protocol.SafetyClassExec &&
		approvalpolicy.CatastrophicShellCommand(subject)
	if policyPrompt || hookPrompt || catastrophic {
		query := approvalpolicy.Query{
			SessionID: tool.scope.SessionID, ProjectDir: tool.scope.Workspace,
			Tool: invocation.Name(), Subject: subject,
		}
		remembered, matched, err := tool.policy.Decide(ctx, query)
		if err != nil {
			return "", fmt.Errorf("agenttools: decide remembered approval: %w", err)
		}
		if matched {
			if remembered == approvalpolicy.DecisionDeny {
				return "", errors.New("tool denied by remembered approval policy")
			}
			if !catastrophic {
				contextText := agentexec.RenderLifecycleContext(decision.Contexts)
				return tool.callAndEnrich(ctx, invocation, effective, contextText)
			}
		}
		if tool.autoApproved && policyPrompt && !hookPrompt && !catastrophic {
			contextText := agentexec.RenderLifecycleContext(decision.Contexts)
			return tool.callAndEnrich(ctx, invocation, effective, contextText)
		}
		reasons := make([]string, 0, 2)
		if policyPrompt {
			reasons = append(reasons, approvalReason(tool.safety))
		}
		if catastrophic {
			reasons = append(reasons, "This command can irreversibly destroy system or user data.")
		}
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			reasons = append(reasons, "Lifecycle policy: "+reason)
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "Lifecycle policy requires confirmation.")
		}
		return "", tool.requireApproval(
			invocation,
			effective,
			subject,
			!tool.intrinsicInput && !catastrophic,
			combineApprovalReasons(reasons...),
		)
	}
	contextText := agentexec.RenderLifecycleContext(decision.Contexts)
	return tool.callAndEnrich(ctx, invocation, effective, contextText)
}

func (tool *lifecyclePolicyTool) resumeApproval(
	ctx context.Context,
	invocation agentexec.ToolInvocation,
	continuation agentexec.ToolInputContinuation,
	state lifecycleApprovalState,
) (string, error) {
	var response lifecycleApprovalResponse
	if err := json.Unmarshal(continuation.Response(), &response); err != nil {
		return "", fmt.Errorf("agenttools: decode lifecycle approval: %w", err)
	}
	if response.Decision != protocol.ApprovalApprove && response.Decision != protocol.ApprovalDeny {
		return "", errors.New("agenttools: unknown lifecycle approval decision")
	}
	if response.Remember != nil {
		if !state.Rememberable {
			return "", errors.New("agenttools: lifecycle approval cannot be remembered")
		}
		remember := approvalpolicy.Remember{
			Scope:     rememberScope(response.Remember.Scope),
			SessionID: tool.scope.SessionID, ProjectDir: tool.scope.Workspace,
			Tool: invocation.Name(), Subject: state.Subject,
			Decision: approvalDecision(response.Decision),
		}
		if err := tool.policy.Remember(ctx, remember); err != nil {
			return "", fmt.Errorf("agenttools: remember approval: %w", err)
		}
	}
	if response.Decision == protocol.ApprovalDeny {
		reason := strings.TrimSpace(response.Reason)
		if reason == "" {
			reason = "denied by user"
		}
		return "", fmt.Errorf("tool denied by user: %s", reason)
	}
	approved := state.Arguments
	if response.EditedArgs != nil {
		edited, err := json.Marshal(response.EditedArgs)
		if err != nil {
			return "", fmt.Errorf("agenttools: encode edited approval arguments: %w", err)
		}
		approved = string(edited)
	}
	// An approval can remain durable while project trust or hook files change.
	// Re-evaluate at the effect boundary so a newly-denying policy cannot be
	// bypassed with a continuation created under stale configuration.
	decision, err := tool.before(ctx, invocation, approved)
	if err != nil {
		return "", err
	}
	if decision.Denied() {
		return "", fmt.Errorf("tool denied by lifecycle policy: %s", denialReason(decision))
	}
	effective := approved
	if decision.RewriteArguments != "" {
		effective = decision.RewriteArguments
	}
	if effective != approved {
		subject, err := tool.approvalSubject(effective)
		if err != nil {
			return "", err
		}
		reasons := []string{
			"A lifecycle hook changed the approved arguments; review the final action.",
		}
		if reason := strings.TrimSpace(decision.Reason); reason != "" {
			reasons = append(reasons, "Lifecycle policy: "+reason)
		}
		return "", tool.requireApproval(
			invocation,
			effective,
			subject,
			state.Rememberable,
			combineApprovalReasons(reasons...),
		)
	}
	contextText := agentexec.RenderLifecycleContext(decision.Contexts)
	return tool.callAndEnrich(ctx, invocation, effective, contextText)
}

func (tool *lifecyclePolicyTool) before(
	ctx context.Context,
	invocation agentexec.ToolInvocation,
	arguments string,
) (lifecyclehook.Decision, error) {
	if len(arguments) > lifecyclehook.MaxArgumentsBytes {
		return lifecyclehook.Decision{}, errors.New("agenttools: tool arguments exceed the lifecycle boundary")
	}
	if _, err := decodeLifecycleArguments(arguments); err != nil {
		return lifecyclehook.Decision{}, fmt.Errorf(
			"agenttools: invalid tool arguments at lifecycle boundary: %w",
			err,
		)
	}
	return tool.hooks.Evaluate(ctx, lifecyclehook.Invocation{
		Event:     lifecyclehook.PreToolUse,
		SessionID: tool.scope.SessionID, RunID: tool.scope.RunID,
		Workspace: tool.scope.Workspace,
		Tool: &lifecyclehook.ToolInput{
			Name: invocation.Name(), Arguments: arguments,
		},
	})
}

func (tool *lifecyclePolicyTool) requireApproval(
	invocation agentexec.ToolInvocation,
	arguments string,
	subject string,
	rememberable bool,
	reason string,
) error {
	decoded, err := decodeLifecycleArguments(arguments)
	if err != nil {
		return fmt.Errorf("agenttools: decode lifecycle approval arguments: %w", err)
	}
	tool.scope.Facts.RecordEffectiveToolArguments(invocation.CallID(), decoded)
	risk := protocol.ApprovalRiskMedium
	if tool.safety == protocol.SafetyClassExec || tool.safety == protocol.SafetyClassNetwork {
		risk = protocol.ApprovalRiskHigh
	}
	prompt, err := json.Marshal(agentexec.ToolInputPrompt{
		Kind: "approval", ItemID: agentexec.ToolItemID(tool.scope.RunID, invocation.CallID()),
		Tool: &agentexec.ToolInputInvocation{
			Name: invocation.Name(), Arguments: decoded,
		},
		SafetyClass: tool.safety, Risk: risk,
		Reason: strings.TrimSpace(reason), Rememberable: rememberable,
	})
	if err != nil {
		return err
	}
	state, err := json.Marshal(lifecycleApprovalState{
		Kind:         lifecycleApprovalStateKind,
		Arguments:    arguments,
		Subject:      subject,
		Rememberable: rememberable,
	})
	if err != nil {
		return err
	}
	return agentexec.RequireToolInput(prompt, lifecycleApprovalSchema, state)
}

func (tool *lifecyclePolicyTool) callAndEnrich(
	ctx context.Context,
	invocation agentexec.ToolInvocation,
	arguments string,
	preContext string,
) (string, error) {
	if decoded, err := decodeLifecycleArguments(arguments); err == nil {
		tool.scope.Facts.RecordEffectiveToolArguments(invocation.CallID(), decoded)
	}
	result, callErr := tool.Tool.Call(ctx, arguments)
	if errors.Is(callErr, agentexec.ErrToolInputRequired) ||
		errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
		return result, callErr
	}
	boundedResult, resultTruncated := boundedLifecycleText(
		result,
		lifecyclehook.MaxResultBytes,
	)
	errorText := ""
	if callErr != nil {
		errorText, _ = boundedLifecycleText(callErr.Error(), lifecyclehook.MaxReasonBytes)
	}
	post := tool.hooks.EvaluateBestEffort(ctx, lifecyclehook.Invocation{
		Event:     lifecyclehook.PostToolUse,
		SessionID: tool.scope.SessionID, RunID: tool.scope.RunID,
		Workspace: tool.scope.Workspace,
		Tool: &lifecyclehook.ToolInput{
			Name: invocation.Name(), Arguments: arguments,
			Result: boundedResult, Error: errorText,
			ResultTruncated: resultTruncated,
		},
	})
	contextText := joinLifecycleContext(
		preContext,
		agentexec.RenderLifecycleContext(post.Contexts),
	)
	if contextText == "" {
		return result, callErr
	}
	wrapped := "<lyra-lifecycle-context>\n" + contextText + "\n</lyra-lifecycle-context>"
	if callErr != nil {
		return result, fmt.Errorf("%w\n\n%s", callErr, wrapped)
	}
	if result == "" {
		return wrapped, nil
	}
	return result + "\n\n" + wrapped, nil
}

func decodeLifecycleArguments(value string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("arguments must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("arguments contain multiple JSON values")
		}
		return nil, err
	}
	return object, nil
}

func (tool *lifecyclePolicyTool) approvalSubject(arguments string) (string, error) {
	decoded, err := decodeLifecycleArguments(arguments)
	if err != nil {
		return "", fmt.Errorf("agenttools: decode approval subject: %w", err)
	}
	if tool.safety == protocol.SafetyClassExec {
		if command, ok := decoded["command"].(string); ok {
			return command, nil
		}
	}
	if tool.paths == nil {
		return "", nil
	}
	paths, err := tool.paths.MutationPaths(arguments)
	if err != nil {
		return "", fmt.Errorf("agenttools: derive approval subject: %w", err)
	}
	paths = slices.DeleteFunc(paths, func(value string) bool { return value == "" })
	sort.Strings(paths)
	paths = slices.Compact(paths)
	return strings.Join(paths, "\n"), nil
}

func effectFromSafety(safety protocol.SafetyClass) approvalpolicy.Effect {
	switch safety {
	case protocol.SafetyClassWrite:
		return approvalpolicy.EffectWrite
	case protocol.SafetyClassExec:
		return approvalpolicy.EffectExec
	case protocol.SafetyClassNetwork:
		return approvalpolicy.EffectNetwork
	default:
		return approvalpolicy.EffectSafe
	}
}

func approvalDecision(value protocol.ApprovalDecision) approvalpolicy.Decision {
	if value == protocol.ApprovalApprove {
		return approvalpolicy.DecisionAllow
	}
	return approvalpolicy.DecisionDeny
}

func rememberScope(value protocol.RememberScopeKind) approvalpolicy.Scope {
	switch value {
	case protocol.RememberSession:
		return approvalpolicy.ScopeSession
	case protocol.RememberProject:
		return approvalpolicy.ScopeProject
	case protocol.RememberGlobal:
		return approvalpolicy.ScopeGlobal
	default:
		return approvalpolicy.Scope(value)
	}
}

func approvalReason(safety protocol.SafetyClass) string {
	switch safety {
	case protocol.SafetyClassWrite:
		return "This tool changes workspace files."
	case protocol.SafetyClassExec:
		return "This tool executes a local command."
	case protocol.SafetyClassNetwork:
		return "This tool accesses the network."
	default:
		return "This tool requires confirmation."
	}
}

func combineApprovalReasons(values ...string) string {
	reason := strings.Join(values, "\n")
	reason, _ = boundedLifecycleText(reason, lifecyclehook.MaxReasonBytes)
	return reason
}

func denialReason(decision lifecyclehook.Decision) string {
	if reason := strings.TrimSpace(decision.Reason); reason != "" {
		return reason
	}
	return "blocked by lifecycle policy"
}

func boundedLifecycleText(value string, limit int) (string, bool) {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func joinLifecycleContext(values ...string) string {
	values = slices.DeleteFunc(values, func(value string) bool {
		return strings.TrimSpace(value) == ""
	})
	return strings.Join(values, "\n\n")
}

var _ toolcontract.WrappingTool = (*lifecyclePolicyTool)(nil)
