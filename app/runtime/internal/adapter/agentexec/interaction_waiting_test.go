package agentexec

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	infraexec "github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

func TestInteractionExecutorRestoresWaitingTreeAndDeliversSemanticAnswer(t *testing.T) {
	workspace := t.TempDir()
	var (
		toolCalls int
		requests  [][]chat.Message
		mu        sync.Mutex
	)
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask for one value before completing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := interactioninput.Require(ctx, "question.ask", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value?"}},
			},
		})
		if err != nil {
			return "", err
		}
		toolCalls++
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "ask", Arguments: `{}`}, 2, 1),
		interactionUsageTextResponse("completed", 3, 1),
	}}
	recordingModel := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		mu.Lock()
		requests = append(requests, cloneChatMessages(request.Messages))
		mu.Unlock()
		return model.Call(ctx, request)
	})
	executor := newObservedTestInteractionExecutor(t, recordingModel, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{question}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	first, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if len(payloadsOf[runs.ToolCallStarted](first)) != 1 || toolCalls != 0 {
		t.Fatalf("before answer starts=%d Tool calls=%d", len(payloadsOf[runs.ToolCallStarted](first)), toolCalls)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}

	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)

	restoredWaiting, err := executor.RestoreWaitingExecution(t.Context(), continuation)
	if err != nil {
		t.Fatalf("RestoreWaitingExecution: %v", err)
	}
	if restoredWaiting != ref {
		t.Fatalf("restored waiting ref = %+v, want %+v", restoredWaiting, ref)
	}
	if _, err := executor.RestoreWaitingExecution(t.Context(), continuation); !errors.Is(err, runs.ErrExecutionClaimed) {
		t.Fatalf("second RestoreWaitingExecution error = %v, want ErrExecutionClaimed", err)
	}
	if err := executor.Release(t.Context(), restoredWaiting); err != nil {
		t.Fatal(err)
	}
	restored, err := executor.StageContinuation(t.Context(), continuation)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	resumedEvents := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	answers := []runs.InterruptAnswer{{
		InterruptItemID: "item_question", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}},
	}}
	if err := executor.BeginContinuation(
		t.Context(), restored, answers, []interrupt.Kind{interrupt.Approval},
	); err == nil {
		t.Fatal("BeginContinuation accepted capabilities that differ from staging")
	}
	if err := executor.BeginContinuation(t.Context(), restored, answers, []interrupt.Kind{interrupt.Question}); err != nil {
		t.Fatal(err)
	}
	events := <-resumedEvents
	if toolCalls != 1 {
		t.Fatalf("Tool calls after answer = %d, want 1", toolCalls)
	}
	completed := payloadsOf[runs.AssistantMessageCompleted](events)
	if len(completed) != 1 || completed[0].Message.Text() != "completed" {
		t.Fatalf("completion = %#v", completed)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || ended[0].Usage == nil || ended[0].Usage.Steps != 2 {
		t.Fatalf("terminal = %#v", ended)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 || requests[0][0].Text() != "current question" || requests[1][0].Text() != "current question" {
		t.Fatalf("restored model context = %#v", requests)
	}
	if err := executor.Release(t.Context(), restored); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorRestoresRuntimeAskUserTool(t *testing.T) {
	workspace := t.TempDir()
	ask, err := builtin.NewAskUser(RequireToolInput)
	if err != nil {
		t.Fatal(err)
	}
	arguments := `{"questions":[{"question":"Which value?"}]}`
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{
			ID: "ask_user_call", Name: toolname.AskUser, Arguments: arguments,
		}, 1, 1),
		interactionUsageTextResponse("continued after the answer", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{ask}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	beforeAnswer, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	starts := payloadsOf[runs.ToolCallStarted](beforeAnswer)
	if len(barrier.Interruptions) != 1 || barrier.Interruptions[0].Interrupt.Question == nil ||
		barrier.Interruptions[0].Interrupt.Question.ToolName != toolname.AskUser ||
		len(starts) != 1 || barrier.Interruptions[0].Interrupt.Question.CallID != starts[0].CallID {
		t.Fatalf("ask_user interruption = %#v", barrier.Interruptions)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)); err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_ask_user", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"chosen"}}},
	}}, []interrupt.Kind{interrupt.Question}); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	if len(payloadsOf[runs.ToolCallFinished](observed)) != 1 ||
		len(payloadsOf[runs.AssistantMessageCompleted](observed)) != 1 {
		t.Fatalf("restored ask_user lifecycle = %#v", observed)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorRestoresInteractiveApprovalWithoutRepeatingPolicyOrHook(t *testing.T) {
	workspace := t.TempDir()
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "mutate", Description: "Perform an approved mutation.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "mutated", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "mutate_call", Name: "mutate", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("approved", 1, 1),
	}}
	hooks := &recordingInteractionHooks{}
	approvals := &promptingInteractionAuthorizer{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: approvals, ToolHooks: hooks,
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	first, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if len(payloadsOf[runs.ToolCallStarted](first)) != 0 || toolCalls != 0 || hooks.before != 1 || approvals.planned != 1 {
		t.Fatalf("before approval starts=%d calls=%d hooks=%d plans=%d", len(payloadsOf[runs.ToolCallStarted](first)), toolCalls, hooks.before, approvals.planned)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)

	if _, err := executor.StageContinuation(t.Context(), continuation); err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Approved: true},
	}}, []interrupt.Kind{interrupt.Approval}); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	if toolCalls != 1 || hooks.before != 1 || hooks.after != 1 || approvals.planned != 1 || approvals.resolved != 1 {
		t.Fatalf("after approval calls=%d before=%d after=%d plans=%d resolves=%d events=%#v", toolCalls, hooks.before, hooks.after, approvals.planned, approvals.resolved, observed)
	}
	if len(payloadsOf[runs.ToolCallStarted](observed)) != 1 || len(payloadsOf[runs.ToolCallFinished](observed)) != 1 {
		t.Fatalf("approved Tool lifecycle = %#v", observed)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancellationStopsApprovedInflightTool(t *testing.T) {
	workspace := t.TempDir()
	toolStarted := make(chan struct{})
	toolReturned := make(chan struct{})
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "mutate", Description: "Perform an approved mutation until canceled.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		close(toolStarted)
		<-ctx.Done()
		close(toolReturned)
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "mutate_call", Name: "mutate", Arguments: `{}`}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: &promptingInteractionAuthorizer{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)
	if _, err := executor.StageContinuation(t.Context(), continuation); err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID: binding.RequestID, Resolution: interrupt.Resolution{Approved: true},
	}}, []interrupt.Kind{interrupt.Approval}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolStarted:
	case <-time.After(time.Second):
		t.Fatal("approved Tool did not start")
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolReturned:
	case <-time.After(time.Second):
		t.Fatal("approved Tool context was not canceled")
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancellationStopsApprovedForegroundShell(t *testing.T) {
	workspace := t.TempDir()
	shells := infraexec.NewShells(nil, false)
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})
	shellTools, err := builtin.BuildShell(shells, workspace)
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{
			ID: "shell_call", Name: toolname.Shell,
			Arguments: `{"command":"sleep 30","description":"Wait until canceled"}`,
		}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: shellTools}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: &promptingInteractionAuthorizer{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Approval}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	continuation := rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Approval}},
	)
	if _, err := executor.StageContinuation(t.Context(), continuation); err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_approval", MemberID: binding.MemberID,
		RequestID: binding.RequestID, Resolution: interrupt.Resolution{Approved: true},
	}}, []interrupt.Kind{interrupt.Approval}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for len(shells.RunningForSession(start.SessionID)) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if running := shells.RunningForSession(start.SessionID); len(running) != 1 {
		t.Fatalf("running shells = %#v, want one approved foreground command", running)
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
	if running := shells.RunningForSession(start.SessionID); len(running) != 0 {
		t.Fatalf("approved foreground shell survived cancellation: %#v", running)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorPreservesDeferredAdvertisementAcrossWaitingRestore(t *testing.T) {
	workspace := t.TempDir()
	hidden, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "hidden_lookup", Description: "Read a deferred value.",
	}, func(context.Context, struct{}) (string, error) { return "hidden", nil })
	if err != nil {
		t.Fatal(err)
	}
	search, err := toolset.NewDiscovery([]toolcontract.Tool{hidden})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask before continuing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := interactioninput.Require(ctx, "question.after-search", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
			},
		})
		if err != nil {
			return "", err
		}
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &advertisementWaitingModel{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{search, question}, Deferred: []toolcontract.Tool{hidden},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
		barrier.Checkpoint,
		ref.ExecutorID,
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)); err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	binding := barrier.Interruptions[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: "item_question", MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"yes"}}},
	}}, []interrupt.Kind{interrupt.Question}); err != nil {
		t.Fatal(err)
	}
	observed := <-events
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal = %#v", ended)
	}
	if !model.restoredManifestIncludedHidden {
		t.Fatal("restored model request omitted the previously advertised Tool")
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}

type advertisementWaitingModel struct {
	mu                             sync.Mutex
	calls                          int
	restoredManifestIncludedHidden bool
}

func (model *advertisementWaitingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	switch model.calls {
	case 1:
		return interactionToolResponse(chat.ToolCall{
			ID: "discover", Name: "search_tools", Arguments: `{"query":"select:hidden_lookup"}`,
		}, 1, 1), nil
	case 2:
		return interactionToolResponse(chat.ToolCall{ID: "ask", Name: "ask", Arguments: `{}`}, 1, 1), nil
	case 3:
		for _, definition := range request.Tools {
			if definition.Name == "hidden_lookup" {
				model.restoredManifestIncludedHidden = true
			}
		}
		if !model.restoredManifestIncludedHidden {
			return nil, errors.New("deferred advertisement was lost across restore")
		}
		return interactionUsageTextResponse("done", 1, 1), nil
	default:
		return nil, errors.New("unexpected model call")
	}
}

type promptingInteractionAuthorizer struct {
	planned  int
	resolved int
}

func rootInteractionWaitingContinuation(
	checkpoint runs.ExecutorCheckpoint,
	executorID string,
	capabilities run.Capabilities,
) runs.WaitingContinuation {
	const rootRunID = "run_root"

	metrics := run.Metrics{}
	if len(checkpoint.Usage.Models) > 0 {
		total, err := checkpoint.Usage.Total()
		if err != nil {
			panic(err)
		}
		usage := &accounting.Usage{
			Total: accounting.Totals{
				InputTokens: total.PromptTokens, OutputTokens: total.CompletionTokens,
				ReasoningTokens: total.ReasoningTokens, CacheReadTokens: total.CacheReadTokens,
				CacheWriteTokens: total.CacheWriteTokens, CostUSD: new(total.CostUSD),
			},
			ByModel: make(map[string]accounting.Totals, len(checkpoint.Usage.Models)),
		}
		for _, model := range checkpoint.Usage.Models {
			usage.ByModel[model.Model] = accounting.Totals{
				InputTokens: model.PromptTokens, OutputTokens: model.CompletionTokens,
				ReasoningTokens: model.ReasoningTokens, CacheReadTokens: model.CacheReadTokens,
				CacheWriteTokens: model.CacheWriteTokens, CostUSD: new(model.CostUSD),
			}
		}
		metrics, err = run.NewMetrics(usage, total.Calls, 0)
		if err != nil {
			panic(err)
		}
	}
	return runs.WaitingContinuation{
		SessionID: checkpoint.Scope.SessionID, ExecutorID: executorID, RootRunID: rootRunID,
		Members: []runs.WaitingMember{{
			RunID: rootRunID, MemberID: checkpoint.RootMemberID,
			ModelSelection: checkpoint.ModelSelection, Metrics: metrics,
		}},
		Checkpoint: checkpoint, Capabilities: capabilities,
		ChildRunAdmissionEnabled: capabilities.ChildRuns,
	}
}

func (authorizer *promptingInteractionAuthorizer) AuthorizeTool(
	_ context.Context,
	request ToolAuthorizationRequest,
) (ToolAuthorizationDecision, error) {
	authorizer.planned++
	prompt := runs.ApprovalPrompt{
		CallID: request.CallID, ToolName: request.ToolName, Arguments: request.Arguments.Canonical(),
		SafetyClass: request.SafetyClass, Risk: domaintool.RiskHigh,
		Reason: "This Tool changes external state.", Rememberable: true,
	}
	return ToolAuthorizationDecision{Approval: &prompt}, nil
}

func (authorizer *promptingInteractionAuthorizer) ResolveToolApproval(
	_ context.Context,
	request ToolAuthorizationRequest,
	_ runs.ApprovalPrompt,
	resolution interrupt.Resolution,
) (ToolAuthorizationDecision, error) {
	authorizer.resolved++
	if !resolution.Approved {
		return ToolAuthorizationDecision{Denied: true, Reason: "denied by user"}, nil
	}
	if resolution.Arguments == "" {
		return ToolAuthorizationDecision{}, nil
	}
	arguments, err := domaintool.ParseArguments(resolution.Arguments)
	if err != nil {
		return ToolAuthorizationDecision{}, err
	}
	if request.ToolName == "" {
		return ToolAuthorizationDecision{}, errors.New("missing Tool identity")
	}
	return ToolAuthorizationDecision{EffectiveArguments: &arguments}, nil
}

func TestInteractionExecutorRejectsInvalidWaitingRecoveryFacts(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	for _, test := range []struct {
		name   string
		mutate func(*runs.ExecutorCheckpoint)
	}{
		{name: "corrupt payload", mutate: func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.Payload = []byte(`{"schema_version":4}`)
		}},
		{name: "wrong build", mutate: func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.BuildID = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		}},
		{name: "missing workspace", mutate: func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.Scope.CWD = workspace + "/gone"
		}},
		{name: "isolated workspace", mutate: func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.Scope.Isolated = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := checkpoint.Clone()
			test.mutate(&candidate)
			executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				return nil, errors.New("model must not be called while restoring")
			}), InteractionExecutorConfig{})
			continuation := rootInteractionWaitingContinuation(
				candidate,
				"exec_restore",
				run.Capabilities{},
			)

			_, err := executor.StageContinuation(t.Context(), continuation)
			if !errors.Is(err, runs.ErrExecutorStateLost) {
				t.Fatalf("StageContinuation error = %v, want ErrExecutorStateLost", err)
			}
		})
	}
	t.Run("wrong deployment", func(t *testing.T) {
		client, err := chatclient.New(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("model must not be called while restoring")
		}), chatclient.Config{})
		if err != nil {
			t.Fatal(err)
		}
		executor, err := NewInteractionExecutor(InteractionExecutorConfig{
			DefaultClient: client, BuildID: interactionTestBuildID,
			ImplementationIdentity: "interaction-observation-test-build",
			ConfigurationIdentity:  "different-deployment-configuration",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
			checkpoint,
			"exec_restore",
			run.Capabilities{},
		))
		if !errors.Is(err, runs.ErrExecutorStateLost) {
			t.Fatalf("StageContinuation error = %v, want ErrExecutorStateLost", err)
		}
	})
	t.Run("isolated workspace cannot be overridden", func(t *testing.T) {
		candidate := checkpoint.Clone()
		candidate.Scope.Isolated = true
		executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("model must not be called while restoring")
		}), InteractionExecutorConfig{
			RestoreScopeValidator: restoreScopeValidatorFunc(func(context.Context, runs.ExecutionScope) error {
				return nil
			}),
		})
		_, err := executor.StageContinuation(t.Context(), rootInteractionWaitingContinuation(
			candidate,
			"exec_restore",
			run.Capabilities{},
		))
		if !errors.Is(err, runs.ErrExecutorStateLost) {
			t.Fatalf("StageContinuation error = %v, want ErrExecutorStateLost", err)
		}
	})
}

func TestInteractionExecutorProbesWaitingCheckpointThroughExactRestorePath(t *testing.T) {
	workspace := t.TempDir()
	checkpoint := captureInteractionQuestionCheckpoint(t, workspace)
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("model must not be called while probing a waiting checkpoint")
	}), InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	continuation := rootInteractionWaitingContinuation(
		checkpoint,
		"exec_probe",
		run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
	)
	resumable, err := executor.CanResumeWaitingExecution(t.Context(), continuation)
	if err != nil || !resumable {
		t.Fatalf("CanResumeWaitingExecution = %t, %v, want true", resumable, err)
	}

	foreign := continuation
	foreign.Members = append([]runs.WaitingMember(nil), continuation.Members...)
	foreign.Members[0].MemberID = "interaction:foreign-member"
	resumable, err = executor.CanResumeWaitingExecution(t.Context(), foreign)
	if err != nil || resumable {
		t.Fatalf("foreign CanResumeWaitingExecution = %t, %v, want false", resumable, err)
	}
}

func TestInteractionExecutorDoesNotCheckpointOrReplayUnknownEffect(t *testing.T) {
	workspace := t.TempDir()
	var calls int
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("externally completed", 2, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		UnknownEffectPollInterval: 5 * time.Millisecond,
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	unknownReady := make(chan struct{})
	go func() {
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				var commitErr error
				if _, completed := commit.Fact.(runs.ModelCallCompleted); completed {
					commitErr = errors.New("final projection is indeterminate")
				}
				commit.Complete(commitErr)
			}
			if _, unknown := event.Payload.(runs.UnknownEffectsDetected); unknown {
				close(unknownReady)
				return
			}
		}
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case <-unknownReady:
	case <-time.After(time.Second):
		t.Fatal("unknown Effect was not observed")
	}
	process := session.processHandle()
	if process == nil {
		t.Fatal("unknown Interaction has no Process")
	}
	captureCtx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := session.engine.CaptureTree(captureCtx, process.ID()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CaptureTree with unknown Effect error = %v, want deadline", err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("provider calls = %d, want no replay after checkpoint probe", calls)
	}
}

func TestInteractionExecutorAppliesSteerAtNextModelBoundary(t *testing.T) {
	model := &runtimeSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	<-model.started
	if err := executor.SubmitSteer(t.Context(), ref, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "add evidence",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := executor.SubmitSteer(t.Context(), ref, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "then shorten it",
	}}); err != nil {
		t.Fatal(err)
	}
	close(model.release)
	observed := <-events
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	steers := payloadsOf[runs.SteerMessagesApplied](observed)
	if len(steers) != 1 || len(steers[0].Messages) != 2 ||
		len(steers[0].Messages[0]) != 1 || steers[0].Messages[0][0].Text != "add evidence" ||
		len(steers[0].Messages[1]) != 1 || steers[0].Messages[1][0].Text != "then shorten it" {
		t.Fatalf("steer projections = %#v", steers)
	}
	steerIndex, secondModelIndex := -1, -1
	modelStarts := 0
	for index, event := range observed {
		switch event.Payload.(type) {
		case runs.SteerMessagesApplied:
			steerIndex = index
		case runs.ModelCallStarted:
			modelStarts++
			if modelStarts == 2 {
				secondModelIndex = index
			}
		}
	}
	if steerIndex < 0 || secondModelIndex < 0 || steerIndex >= secondModelIndex {
		t.Fatalf("event order steer=%d second model=%d: %#v", steerIndex, secondModelIndex, observed)
	}
}

type runtimeSteerModel struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

type restoreScopeValidatorFunc func(context.Context, runs.ExecutionScope) error

func (validate restoreScopeValidatorFunc) ValidateRestoreScope(
	ctx context.Context,
	scope runs.ExecutionScope,
) error {
	return validate(ctx, scope)
}

func (model *runtimeSteerModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	model.calls++
	call := model.calls
	model.mu.Unlock()
	if call == 1 {
		if len(request.Messages) != 1 {
			return nil, errors.New("steer reached the in-flight model request")
		}
		close(model.started)
		<-model.release
		return interactionUsageTextResponse("draft", 1, 1), nil
	}
	if len(request.Messages) != 4 || request.Messages[1].Text() != "draft" ||
		request.Messages[2].Role != chat.RoleUser || request.Messages[2].Text() != "add evidence" ||
		request.Messages[3].Role != chat.RoleUser || request.Messages[3].Text() != "then shorten it" {
		return nil, errors.New("steer was not visible at the next model boundary")
	}
	return interactionUsageTextResponse("revised", 1, 1), nil
}

func captureInteractionQuestionCheckpoint(t *testing.T, workspace string) runs.ExecutorCheckpoint {
	t.Helper()
	question := newQuestionCheckpointTool(t)
	executor := newObservedTestInteractionExecutor(t, &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "ask", Arguments: `{}`}, 1, 1),
	}}, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{question}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	return barrier.Checkpoint
}

func newQuestionCheckpointTool(t *testing.T) toolcontract.Tool {
	t.Helper()
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask before completing.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		_, err := interactioninput.Require(ctx, "question.ask", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value?"}},
			},
		})
		return "", err
	})
	if err != nil {
		t.Fatal(err)
	}
	return question
}

func observeInteractionUntilWaiting(
	t *testing.T,
	executor *InteractionExecutor,
	ref runs.ExecutorRef,
	begin func() error,
) ([]runs.ExecutorEvent, runs.TreeInterrupted) {
	t.Helper()
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		events  []runs.ExecutorEvent
		barrier runs.TreeInterrupted
	}
	ready := make(chan result, 1)
	go func() {
		var value result
		sequence(func(event runs.ExecutorEvent) bool {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact
			}
			value.events = append(value.events, event)
			if barrier, waiting := event.Payload.(runs.TreeInterrupted); waiting {
				value.barrier = barrier
				return false
			}
			return true
		})
		ready <- value
	}()
	if err := begin(); err != nil {
		t.Fatal(err)
	}
	value := <-ready
	if len(value.barrier.Interruptions) != 1 {
		t.Fatalf("waiting barrier = %#v", value.barrier)
	}
	return value.events, value.barrier
}

func collectInteractionEvents(sequence func(func(runs.ExecutorEvent) bool)) <-chan []runs.ExecutorEvent {
	ready := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		sequence(func(event runs.ExecutorEvent) bool {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact
			}
			events = append(events, event)
			return true
		})
		ready <- events
	}()
	return ready
}
