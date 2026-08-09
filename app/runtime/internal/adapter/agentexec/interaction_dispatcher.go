package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/discovery"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

const authoritativeProjectionTimeout = 15 * time.Second

// interactionDispatcher gives each EffectRequest one independent attempt
// tracker. The inner Interaction Dispatcher still owns protocol decoding and
// definite settlements; this wrapper alone converts a post-external-call
// projection failure into Agent Framework's unknown settlement path.
type interactionDispatcher struct {
	inner   *interaction.Dispatcher
	session *interactionSession
}

func (dispatcher *interactionDispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	emit agent.DeltaEmitter,
) (agent.Settlement, error) {
	attempt := newDispatchAttempt(request.ID())
	settlement, err := dispatcher.inner.Dispatch(withDispatchAttempt(ctx, attempt), request, emit)
	if projectionErr := attempt.indeterminateFailure(); projectionErr != nil {
		dispatcher.session.wakeUnknownReconciliation()
		return agent.Settlement{}, fmt.Errorf(
			"agentexec: authoritative projection failed after external Effect %s: %w",
			request.ID(), projectionErr,
		)
	}
	if err != nil && attempt.crossedExternalBoundary() {
		// The inner Dispatcher already returns an indeterminate error to Engine.
		// Wake the direct path as well; the periodic public-state reconciliation
		// remains the loss-tolerant backstop.
		dispatcher.session.wakeUnknownReconciliation()
	}
	return settlement, err
}

func (dispatcher *interactionDispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	return dispatcher.inner.ReplayPolicy(effect)
}

type observedInteractionModel struct {
	inner   *chatclient.Client
	session *interactionSession
}

func (model *observedInteractionModel) Call(
	ctx context.Context,
	request *corechat.Request,
) (*corechat.Response, error) {
	invocation, attempt, callID, err := model.begin(ctx)
	if err != nil {
		return nil, err
	}
	if err := attempt.beginExternalCall(); err != nil {
		return nil, err
	}
	response, err := model.inner.Call(ctx, request)
	if err != nil {
		if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		return response, err
	}
	if response == nil {
		responseErr := errors.New("agentexec: model returned no response")
		if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(responseErr, projectionErr)
		}
		return nil, responseErr
	}
	if err := response.Validate(); err != nil {
		if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
			attempt.recordProjectionFailure(projectionErr)
			return nil, errors.Join(err, projectionErr)
		}
		return response, err
	}
	if err := model.complete(ctx, invocation, callID, response); err != nil {
		attempt.recordProjectionFailure(err)
		return nil, err
	}
	return response, nil
}

func (model *observedInteractionModel) Stream(
	ctx context.Context,
	request *corechat.Request,
) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		invocation, attempt, callID, err := model.begin(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		if err := attempt.beginExternalCall(); err != nil {
			yield(nil, err)
			return
		}
		var accumulated corechat.ResponseAccumulator
		for chunk, streamErr := range model.inner.Stream(ctx, request) {
			if streamErr != nil {
				if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
					attempt.recordProjectionFailure(projectionErr)
					streamErr = errors.Join(streamErr, projectionErr)
				}
				yield(nil, streamErr)
				return
			}
			if err := accumulated.Add(chunk); err != nil {
				if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
					attempt.recordProjectionFailure(projectionErr)
					err = errors.Join(err, projectionErr)
				}
				yield(nil, err)
				return
			}
			if !yield(chunk, nil) {
				if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
					attempt.recordProjectionFailure(projectionErr)
				}
				return
			}
		}
		response := accumulated.Response()
		if response == nil {
			if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
				attempt.recordProjectionFailure(projectionErr)
				yield(nil, projectionErr)
				return
			}
			yield(nil, errors.New("agentexec: model stream completed without a response"))
			return
		}
		if err := response.Validate(); err != nil {
			if projectionErr := model.fail(ctx, invocation, callID); projectionErr != nil {
				attempt.recordProjectionFailure(projectionErr)
				err = errors.Join(err, projectionErr)
			}
			yield(nil, err)
			return
		}
		if err := model.complete(ctx, invocation, callID, response); err != nil {
			attempt.recordProjectionFailure(err)
			yield(nil, err)
		}
	}
}

func (model *observedInteractionModel) fail(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
) error {
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	return model.session.commitFact(
		projectionCtx,
		model.session.executorMember(invocation.Relation()),
		runs.ModelCallFailed{CallID: callID},
	)
}

func (model *observedInteractionModel) begin(
	ctx context.Context,
) (interaction.ModelInvocation, *dispatchAttempt, string, error) {
	invocation, ok := interaction.ModelInvocationFromContext(ctx)
	if !ok {
		return interaction.ModelInvocation{}, nil, "", errors.New("agentexec: model call has no Interaction attribution")
	}
	attempt, err := dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	callID := modelInvocationID(invocation)
	if _, err := model.session.reconcileCompletedDelegateChildren(ctx); err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	member := model.session.executorMember(invocation.Relation())
	if err := model.session.commitPendingSteers(ctx, member); err != nil {
		return interaction.ModelInvocation{}, nil, "", err
	}
	if err := model.session.commitFact(ctx, member, runs.ModelCallStarted{CallID: callID}); err != nil {
		return interaction.ModelInvocation{}, nil, "", fmt.Errorf("agentexec: commit model call start: %w", err)
	}
	return invocation, attempt, callID, nil
}

func (model *observedInteractionModel) complete(
	ctx context.Context,
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) error {
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return errors.New("agentexec: completed model call has no assistant message")
	}
	fact, err := model.session.accountModelCall(invocation, callID, response)
	if err != nil {
		return err
	}
	projectionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
	defer cancel()
	if err := model.session.commitFact(
		projectionCtx, model.session.executorMember(invocation.Relation()), fact,
	); err != nil {
		return err
	}
	if !invocation.Relation().IsRoot() {
		model.session.recordCommittedModelReply(invocation.Relation().ProcessID(), fact.Message)
	}
	return model.session.registerDelegateCalls(invocation, choice.Message)
}

type observedInteractionTool struct {
	inner       toolcontract.Tool
	session     *interactionSession
	interpreter InteractionToolInterpreter
	presenter   InteractionToolPresenter
	authorizer  InteractionToolAuthorizer
	hooks       InteractionToolHooks
	offloader   toolResultOffloader
	offloadAt   int
	readTool    string
	start       runs.RootExecutionStart
}

func (observed *observedInteractionTool) Definition() corechat.ToolDefinition {
	return observed.inner.Definition()
}

func (observed *observedInteractionTool) Unwrap() toolcontract.Tool { return observed.inner }

func (observed *observedInteractionTool) Call(ctx context.Context, rawArguments string) (string, error) {
	invocation, ok := interaction.ToolInvocationFromContext(ctx)
	if !ok {
		return "", errors.New("agentexec: Tool call has no Interaction attribution")
	}
	attempt, err := dispatchAttemptFrom(ctx, invocation.EffectID())
	if err != nil {
		return "", err
	}
	call := invocation.ToolCall()
	if call.Name != observed.Definition().Name || call.Arguments != rawArguments {
		return "", errors.New("agentexec: Tool invocation differs from its bound executable")
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return "", fmt.Errorf("agentexec: parse Tool %q arguments: %w", call.Name, err)
	}
	callID := toolInvocationID(invocation)
	ctx = interactioninput.WithCapabilities(ctx, observed.start.InterruptKinds)
	arguments, denied, denialReason, err := observed.prepare(ctx, callID, call.Name, arguments)
	if err != nil {
		return "", err
	}
	rawArguments = arguments.Canonical()
	member := observed.session.executorMember(invocation.Relation())
	start := runs.ToolCallStarted{
		CallID: callID, ModelCallSequence: invocation.ModelCallSequence(),
		ToolCallIndex: invocation.ToolCallIndex(), SourceCallID: call.ID, ToolName: call.Name,
		Arguments: rawArguments, Activity: observed.activity(call.Name, arguments),
		SafetyClass: observed.interpreter.SafetyClass(call.Name),
	}
	if err := observed.session.commitFact(ctx, member, start); err != nil {
		return "", fmt.Errorf("agentexec: commit Tool call start: %w", err)
	}
	observed.session.recordToolCall()
	if denied {
		if denialReason == "" {
			denialReason = "tool call denied by policy"
		}
		end := observed.finishedFact(callID, arguments, denialReason, nil, nil, errors.New(denialReason))
		end.Problem = &transcript.Problem{
			Kind: transcript.DeniedByUserProblem, Scope: transcript.ToolProblem,
		}
		if err := observed.session.commitFact(ctx, member, end); err != nil {
			return "", fmt.Errorf("agentexec: commit denied Tool result: %w", err)
		}
		observed.session.recordToolOutcome(call.Name, arguments, denialReason)
		return denialReason, nil
	}
	ctx = discovery.WithToolAdvertiser(ctx, func(names ...string) error {
		return interaction.AdvertiseTools(ctx, names...)
	})
	if err := attempt.beginExternalCall(); err != nil {
		return "", err
	}
	output, callErr := observed.inner.Call(ctx, rawArguments)
	var inputRequired *interaction.ToolInputRequiredError
	if errors.As(callErr, &inputRequired) {
		// Tool input is an Interaction control boundary, not a failed external
		// call. The started fact remains open so the Run barrier can carry it as
		// a drained Tool; the restored invocation will commit the sole final fact
		// after consuming the semantic response Signal.
		return "", callErr
	}
	modelOutput, offload := observed.offload(ctx, call.Name, output, callErr)
	end := observed.finishedFact(
		callID,
		arguments,
		modelOutput,
		offload,
		observed.mutatedPaths(arguments, callErr),
		callErr,
	)
	// A later concurrent Tool may finish before an earlier model-declared call.
	// Its commit receipt intentionally waits for the canonical durable prefix;
	// the Effect context and executor release own that wait, not an arbitrary local
	// timeout that could misclassify a healthy long-running sibling as unknown.
	projectionCtx := context.WithoutCancel(ctx)
	commitErr := observed.session.commitFact(projectionCtx, member, end)
	if commitErr != nil {
		attempt.recordProjectionFailure(commitErr)
		return "", fmt.Errorf("agentexec: commit Tool result: %w", commitErr)
	}
	outcomeForLoop := modelOutput
	if callErr != nil {
		outcomeForLoop = "error:" + callErr.Error()
	}
	observed.session.recordToolOutcome(call.Name, arguments, outcomeForLoop)
	if observed.interpreter != nil {
		projected, projectionErr := observed.interpreter.ProjectOutcome(
			projectionCtx, observed.start.SessionID, call.Name, callErr == nil,
		)
		if projectionErr != nil {
			trace.SpanFromContext(projectionCtx).RecordError(
				fmt.Errorf("agentexec: project Tool outcome: %w", projectionErr),
			)
		} else if projected != nil {
			// Tool outcome projection is a refetchable live hint (for example a Plan
			// snapshot), not a second settlement fact. The canonical Tool result above
			// is already committed; losing this hint cannot make the Effect unknown.
			observed.session.send(runs.ExecutorEvent{Member: member, Payload: projected})
		}
	}
	if observed.hooks != nil {
		hookCtx, hookCancel := context.WithTimeout(context.WithoutCancel(ctx), authoritativeProjectionTimeout)
		if hookErr := observed.hooks.AfterToolUse(hookCtx, InteractionToolHookInput{
			SessionID: observed.start.SessionID, CWD: observed.start.CWD, CallID: callID,
			ToolName: call.Name, Arguments: arguments, Result: modelOutput, CallError: callErr,
		}); hookErr != nil {
			trace.SpanFromContext(hookCtx).RecordError(
				fmt.Errorf("agentexec: run post-Tool hook: %w", hookErr),
			)
		}
		hookCancel()
	}
	return modelOutput, callErr
}

func (observed *observedInteractionTool) prepare(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
) (tool.Arguments, bool, string, error) {
	continued, resumed, err := interactioninput.Restore(ctx)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	if resumed {
		return observed.resumePreparedTool(ctx, callID, name, continued)
	}
	forceApproval := false
	if observed.hooks != nil {
		decision, err := observed.hooks.BeforeToolUse(ctx, InteractionToolHookInput{
			SessionID: observed.start.SessionID, CWD: observed.start.CWD,
			CallID: callID, ToolName: name, Arguments: arguments,
		})
		if err != nil {
			return tool.Arguments{}, false, "", fmt.Errorf("agentexec: run pre-Tool hook: %w", err)
		}
		if err := validateHookDecision(decision); err != nil {
			return tool.Arguments{}, false, "", err
		}
		if decision.EffectiveArguments != nil {
			arguments = *decision.EffectiveArguments
		}
		if decision.Denied {
			return arguments, true, decision.Reason, nil
		}
		forceApproval = decision.RequireApproval
	}
	if observed.authorizer == nil || !observed.interpreter.UsesStandardPolicy(name) {
		if forceApproval {
			return arguments, true, "a lifecycle hook requires approval, but approval is unavailable", nil
		}
		return observed.applyDoomLoopBrake(ctx, callID, name, arguments, false, "")
	}
	request, err := observed.authorizationRequest(callID, name, arguments, forceApproval)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	decision, err := observed.authorizer.AuthorizeTool(ctx, request)
	if err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: authorize Tool %q: %w", name, err)
	}
	if err := validateToolAuthorizationDecision(decision); err != nil {
		return tool.Arguments{}, false, "", err
	}
	if decision.EffectiveArguments != nil {
		arguments = *decision.EffectiveArguments
	}
	if decision.Denied {
		return arguments, true, decision.Reason, nil
	}
	if decision.Approval != nil {
		return observed.requestToolApproval(ctx, request, *decision.Approval)
	}
	return observed.applyDoomLoopBrake(ctx, callID, name, arguments, false, "")
}

func (observed *observedInteractionTool) applyDoomLoopBrake(
	ctx context.Context,
	callID string,
	name string,
	arguments tool.Arguments,
	denied bool,
	reason string,
) (tool.Arguments, bool, string, error) {
	if denied || observed.session.repeatedToolOutcome(name, arguments) < interactionDoomLoopThreshold {
		return arguments, denied, reason, nil
	}
	observed.session.resetRepeatedToolOutcome()
	reason = fmt.Sprintf(
		"%q has been called with the same arguments and unchanged result %d times; approve to continue or deny so the agent changes approach",
		name, interactionDoomLoopThreshold,
	)
	if observed.authorizer == nil || !slices.Contains(observed.start.InterruptKinds, interrupt.Approval) {
		return arguments, true, reason, nil
	}
	request, err := observed.authorizationRequest(callID, name, arguments, true)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	return observed.requestToolApproval(ctx, request, runs.ApprovalPrompt{
		CallID: callID, ToolName: name, Arguments: arguments.Canonical(),
		SafetyClass: request.SafetyClass, Risk: tool.RiskHigh, Reason: reason,
	})
}

func (observed *observedInteractionTool) authorizationRequest(
	callID string,
	name string,
	arguments tool.Arguments,
	requireApproval bool,
) (ToolAuthorizationRequest, error) {
	subject, err := observed.interpreter.ApprovalSubject(name, arguments)
	if err != nil {
		return ToolAuthorizationRequest{}, fmt.Errorf("agentexec: derive Tool %q approval subject: %w", name, err)
	}
	autoApproved := false
	if observed.session != nil && observed.session.mcpToolAutoApproved != nil {
		if identity, ok := observed.inner.(interactionMCPToolIdentity); ok {
			server, remote := identity.MCPToolIdentity()
			autoApproved = server != "" && remote != "" && observed.session.mcpToolAutoApproved(server, remote)
		}
	}
	return ToolAuthorizationRequest{
		SessionID: observed.start.SessionID, CWD: observed.start.CWD,
		CallID: callID, ToolName: name, Arguments: arguments,
		SafetyClass:     observed.interpreter.SafetyClass(name),
		ApprovalSubject: subject,
		FileMutation:    fileMutationScope(observed.inner, arguments, observed.start.CWD),
		ShellCommand:    observed.interpreter.ShellCommand(name, arguments.Canonical()),
		AutoApproved:    autoApproved,
		RequireApproval: requireApproval,
	}, nil
}

func (observed *observedInteractionTool) requestToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
) (tool.Arguments, bool, string, error) {
	if !slices.Contains(observed.start.InterruptKinds, interrupt.Approval) {
		return request.Arguments, true, "approval input is unavailable for this Run", nil
	}
	if prompt.CallID == "" {
		prompt.CallID = request.CallID
	}
	pending := runs.Interrupt{Kind: interrupt.Approval, Approval: &prompt}
	if err := pending.Validate(); err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: invalid Tool approval prompt: %w", err)
	}
	if prompt.CallID != request.CallID || prompt.ToolName != request.ToolName ||
		prompt.Arguments != request.Arguments.Canonical() || prompt.SafetyClass != request.SafetyClass {
		return tool.Arguments{}, false, "", errors.New("agentexec: Tool approval prompt differs from its invocation")
	}
	resolution, err := interactioninput.Require(
		ctx,
		interrupt.InterruptKey(interrupt.Approval.String(), request.ToolName, request.Arguments.Canonical()),
		pending,
	)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	return observed.resolveToolApproval(ctx, request, prompt, resolution)
}

func (observed *observedInteractionTool) resumePreparedTool(
	ctx context.Context,
	callID string,
	name string,
	continued interactioninput.Continuation,
) (tool.Arguments, bool, string, error) {
	storedName, storedArguments := continued.Interrupt.Tool()
	if storedName != name {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool input belongs to another Tool")
	}
	arguments, err := tool.ParseArguments(storedArguments)
	if err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: parse continued Tool arguments: %w", err)
	}
	if continued.Interrupt.Kind == interrupt.Question {
		return arguments, false, "", nil
	}
	if continued.Interrupt.Kind != interrupt.Approval || continued.Interrupt.Approval == nil {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool input has an unsupported kind")
	}
	prompt := *continued.Interrupt.Approval
	if prompt.CallID != callID {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool approval call identity changed")
	}
	request, err := observed.authorizationRequest(callID, name, arguments, false)
	if err != nil {
		return tool.Arguments{}, false, "", err
	}
	return observed.resolveToolApproval(ctx, request, prompt, continued.Resolution)
}

func (observed *observedInteractionTool) resolveToolApproval(
	ctx context.Context,
	request ToolAuthorizationRequest,
	prompt runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (tool.Arguments, bool, string, error) {
	if observed.authorizer == nil {
		return tool.Arguments{}, false, "", errors.New("agentexec: continued Tool approval has no authorizer")
	}
	decision, err := observed.authorizer.ResolveToolApproval(ctx, request, prompt, resolution)
	if err != nil {
		return tool.Arguments{}, false, "", fmt.Errorf("agentexec: resolve Tool %q approval: %w", request.ToolName, err)
	}
	if err := validateToolAuthorizationDecision(decision); err != nil {
		return tool.Arguments{}, false, "", err
	}
	arguments := request.Arguments
	if decision.EffectiveArguments != nil {
		arguments = *decision.EffectiveArguments
	}
	return arguments, decision.Denied, decision.Reason, nil
}

func (observed *observedInteractionTool) activity(name string, arguments tool.Arguments) string {
	if observed.presenter != nil {
		if activity := observed.presenter.Activity(name, arguments); activity != "" && activity == strings.TrimSpace(activity) {
			return activity
		}
	}
	return "Calling " + name
}

func (observed *observedInteractionTool) finishedFact(
	callID string,
	arguments tool.Arguments,
	output string,
	offload *toolresult.Ref,
	mutatedPaths []string,
	callErr error,
) runs.ToolCallFinished {
	var result *tool.Result
	outputText := ""
	if output != "" {
		parsed, err := tool.ParseResult([]byte(output))
		if err != nil {
			parsed = tool.StringResult(output)
		}
		if observed.presenter != nil {
			parsed, outputText = observed.presenter.Present(observed.Definition().Name, arguments, parsed)
		}
		result = &parsed
	}
	finished := runs.ToolCallFinished{
		CallID: callID, Arguments: arguments.Canonical(), Result: result,
		Offload: offload, OutputText: outputText, MutatedPaths: slices.Clone(mutatedPaths),
	}
	if callErr != nil {
		finished.Problem = &transcript.Problem{
			Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem,
			Detail: callErr.Error(),
		}
	}
	return finished
}

func (observed *observedInteractionTool) offload(
	ctx context.Context,
	toolName string,
	output string,
	callErr error,
) (string, *toolresult.Ref) {
	if callErr != nil {
		return output, nil
	}
	return evictToolResult(
		ctx,
		observed.offloader,
		observed.offloadAt,
		observed.readTool,
		observed.start.SessionID,
		toolName,
		output,
	)
}

func (observed *observedInteractionTool) mutatedPaths(
	arguments tool.Arguments,
	callErr error,
) []string {
	if callErr != nil {
		return nil
	}
	reporter, ok, err := toolcontract.Capability[interactionFileMutationReporter](observed.inner)
	if err != nil || !ok {
		return nil
	}
	paths, err := reporter.MutationPaths(arguments.Canonical())
	if err != nil {
		return nil
	}
	paths = slices.DeleteFunc(slices.Clone(paths), func(path string) bool { return path == "" })
	slices.Sort(paths)
	return slices.Compact(paths)
}

func modelInvocationID(invocation interaction.ModelInvocation) string {
	return "model:" + invocation.EffectID().String() + ":" + strconv.FormatUint(uint64(invocation.ModelCallSequence()), 10)
}

func toolInvocationID(invocation interaction.ToolInvocation) string {
	return delegatedToolCallID(
		invocation.Relation(), invocation.ModelCallSequence(), invocation.ToolCallIndex(), invocation.ToolCall(),
	)
}

func delegatedToolCallID(
	relation agent.ProcessRelation,
	modelCallSequence uint32,
	toolCallIndex uint32,
	call corechat.ToolCall,
) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(relation.ProcessID().String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.ID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.Name))
	return "tool:" + hex.EncodeToString(digest.Sum(nil)) + ":" + strconv.FormatUint(uint64(toolCallIndex), 10)
}

func basicExecutorMember(relation agent.ProcessRelation) runs.ExecutorMember {
	member := runs.ExecutorMember{MemberID: relation.ProcessID().String()}
	if parentID, child := relation.ParentID(); child {
		member.ParentID = parentID.String()
	}
	return member
}

func wrapInteractionTools(
	manifest toolset.Manifest,
	session *interactionSession,
	config InteractionExecutorConfig,
	start runs.RootExecutionStart,
) (visible []toolcontract.Tool, deferred []toolcontract.Tool) {
	wrap := func(values []toolcontract.Tool) []toolcontract.Tool {
		wrapped := make([]toolcontract.Tool, len(values))
		for index, executable := range values {
			wrapped[index] = &observedInteractionTool{
				inner: executable, session: session, interpreter: config.ToolInterpreter,
				presenter: config.ToolPresenter, authorizer: config.ToolAuthorizer,
				hooks: config.ToolHooks, offloader: config.ToolResultStore,
				offloadAt: config.ToolResultThreshold, readTool: config.ToolResultReaderName,
				start: start,
			}
		}
		return wrapped
	}
	return wrap(manifest.Visible), wrap(manifest.Deferred)
}

func newObservedInteractionClient(
	inner *chatclient.Client,
	session *interactionSession,
) (*chatclient.Client, error) {
	model := &observedInteractionModel{inner: inner, session: session}
	return chatclient.New(model, chatclient.Config{Streamer: model})
}

func validateToolManifest(manifest toolset.Manifest) error {
	seen := make(map[string]string, len(manifest.Visible)+len(manifest.Deferred))
	for _, group := range []struct {
		name   string
		values []toolcontract.Tool
	}{{name: "visible", values: manifest.Visible}, {name: "deferred", values: manifest.Deferred}} {
		name, values := group.name, group.values
		for index, executable := range values {
			if isNilInteractionCapability(executable) {
				return fmt.Errorf("agentexec: %s Interaction Tool[%d] is nil", name, index)
			}
			toolName := executable.Definition().Name
			if strings.TrimSpace(toolName) == "" || toolName != strings.TrimSpace(toolName) {
				return fmt.Errorf("agentexec: %s Interaction Tool[%d] has an invalid name", name, index)
			}
			if prior, duplicate := seen[toolName]; duplicate {
				return fmt.Errorf(
					"agentexec: Interaction Tool %q appears more than once (first in %s, again in %s)",
					toolName,
					prior,
					name,
				)
			}
			seen[toolName] = name
		}
	}
	return nil
}

func isNilInteractionCapability(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func modelUsage(
	response *corechat.Response,
	provider string,
	fallbackModel string,
	pricing accounting.Pricing,
) accounting.ModelUsage {
	servedModel := response.Model
	if servedModel == "" {
		servedModel = fallbackModel
	}
	if servedModel == "" {
		servedModel = "unknown"
	}
	cost := 0.0
	if pricing != nil {
		cost = pricing(provider, servedModel, &response.Usage)
	}
	return accounting.ModelUsage{
		Model: servedModel, TokenUsage: accountingTokenUsage(response.Usage), CostUSD: cost, Calls: 1,
	}
}

func accountingTokenUsage(usage corechat.Usage) accounting.TokenUsage {
	result := accounting.TokenUsage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
	}
	if usage.ReasoningTokens != nil {
		result.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadInputTokens != nil {
		result.CacheReadTokens = *usage.CacheReadInputTokens
	}
	if usage.CacheWriteInputTokens != nil {
		result.CacheWriteTokens = *usage.CacheWriteInputTokens
	}
	return result
}
