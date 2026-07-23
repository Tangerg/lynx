package turn_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestChildToolsShareRootHITLAndHookContract(t *testing.T) {
	tests := []struct {
		name             string
		childTool        string
		childArguments   string
		interruptKinds   []string
		wantInterrupt    runs.InterruptKind
		resolution       interrupts.Resolution
		rewriteArguments string
		wantArguments    string
		wantDenied       bool
		wantDenyReason   string
	}{
		{
			name:             "approval with human edited arguments",
			childTool:        "shell",
			childArguments:   `{"command":"echo original"}`,
			interruptKinds:   []string{"approval"},
			wantInterrupt:    runs.ApprovalInterruptKind,
			resolution:       interrupts.Resolution{Approved: true, Arguments: `{"command":"echo human"}`},
			rewriteArguments: `{"command":"echo hook"}`,
			wantArguments:    `{"command":"echo human"}`,
		},
		{
			name:             "approval denial",
			childTool:        "shell",
			childArguments:   `{"command":"echo original"}`,
			interruptKinds:   []string{"approval"},
			wantInterrupt:    runs.ApprovalInterruptKind,
			resolution:       interrupts.Resolution{Approved: false, Reason: "not this time"},
			rewriteArguments: `{"command":"echo hook"}`,
			wantDenied:       true,
			wantDenyReason:   "not this time",
		},
		{
			name:           "safe child tool",
			childTool:      "glob",
			childArguments: `{"pattern":"*"}`,
		},
		{
			name:           "child question",
			childTool:      "ask_user",
			childArguments: `{"questions":[{"question":"Continue?","options":[{"label":"Yes"},{"label":"No"}]}]}`,
			interruptKinds: []string{"question"},
			wantInterrupt:  runs.QuestionInterruptKind,
			resolution: interrupts.Resolution{
				Approved: true,
				Answer:   map[string][]string{runs.QuestionFieldID(0): {"Yes"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := &hookCommandRecorder{rewriteTool: test.childTool, rewriteArguments: test.rewriteArguments}
			bound := hooks.NewBound([]hooks.Hook{
				{Event: hooks.PreToolUse, Command: "record", Source: "test"},
				{Event: hooks.PostToolUse, Command: "record", Source: "test"},
			}, hooks.NewRunner(recorder, nil))
			policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
			dispatcher := buildB8Dispatcher(t, &childToolModel{
				defaults:       &chat.Options{Model: "b8-child-hitl"},
				childTool:      test.childTool,
				childArguments: test.childArguments,
			}, policy, staticHookResolver{bound: bound})

			handle, err := dispatcher.StartTurn(t.Context(), turn.StartTurnRequest{
				SessionID:      "sess-b8-" + strings.ReplaceAll(test.name, " ", "-"),
				Message:        "delegate this work",
				Cwd:            t.TempDir(),
				InterruptKinds: test.interruptKinds,
			})
			if err != nil {
				t.Fatalf("StartTurn: %v", err)
			}
			events, err := dispatcher.Events(t.Context(), handle)
			if err != nil {
				t.Fatalf("Events: %v", err)
			}

			var (
				interruptCount int
				childStart     *runs.ToolCallStart
				childEnd       *runs.ToolCallEnd
				endReason      execution.Outcome
			)
			for event := range events {
				switch event := event.(type) {
				case runs.TurnInterrupted:
					interruptCount++
					if len(event.Interrupts) != 1 {
						t.Fatalf("interrupts = %#v", event.Interrupts)
					}
					pending := event.Interrupts[0]
					if pending.Kind != test.wantInterrupt {
						t.Fatalf("interrupt kind = %q, want %q", pending.Kind, test.wantInterrupt)
					}
					toolName, _ := pending.Tool()
					if toolName != test.childTool {
						t.Fatalf("interrupt tool = %q, want child %q (task must not be gated)", toolName, test.childTool)
					}
					if err := dispatcher.Resume(t.Context(), handle, test.resolution, test.interruptKinds); err != nil {
						t.Fatalf("Resume: %v", err)
					}
				case runs.ToolCallStart:
					if event.ToolName == test.childTool {
						copy := event
						childStart = &copy
					}
				case runs.ToolCallEnd:
					if childStart != nil && event.CallID == childStart.CallID {
						copy := event
						childEnd = &copy
					}
				case runs.TurnEnd:
					endReason = event.Reason
				}
			}

			if test.wantInterrupt == "" && interruptCount != 0 {
				t.Fatalf("safe child interrupt count = %d, want 0", interruptCount)
			}
			if test.wantInterrupt != "" && interruptCount != 1 {
				t.Fatalf("interrupt count = %d, want 1", interruptCount)
			}
			if endReason != execution.OutcomeCompleted {
				t.Fatalf("turn end = %q, want completed", endReason)
			}
			if childStart == nil || childEnd == nil {
				t.Fatalf("child lifecycle start/end = %#v / %#v", childStart, childEnd)
			}
			if test.wantArguments != "" && childStart.Arguments != test.wantArguments {
				t.Fatalf("child arguments = %s, want %s", childStart.Arguments, test.wantArguments)
			}
			if childEnd.Denied != test.wantDenied {
				t.Fatalf("child denied = %v, want %v", childEnd.Denied, test.wantDenied)
			}
			if test.wantDenyReason != "" {
				result, ok := childEnd.Result.String()
				if !ok || result != test.wantDenyReason {
					t.Fatalf("child deny result = %#v, want %q", childEnd.Result, test.wantDenyReason)
				}
			}
			if got := recorder.count(hooks.PreToolUse, test.childTool); got != 1 {
				t.Fatalf("PreToolUse(%s) count = %d, want 1", test.childTool, got)
			}
			if got := recorder.count(hooks.PostToolUse, test.childTool); got != 1 {
				t.Fatalf("PostToolUse(%s) count = %d, want 1", test.childTool, got)
			}
			if got := recorder.count(hooks.PreToolUse, "task"); got != 0 {
				t.Fatalf("PreToolUse(task) count = %d, want 0", got)
			}
			if got := recorder.count(hooks.PostToolUse, "task"); got != 0 {
				t.Fatalf("PostToolUse(task) count = %d, want 0", got)
			}
		})
	}
}

func TestChildCanSuspendTwiceOnTheSameRun(t *testing.T) {
	recorder := &hookCommandRecorder{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.PreToolUse, Command: "record", Source: "test"},
		{Event: hooks.PostToolUse, Command: "record", Source: "test"},
	}, hooks.NewRunner(recorder, nil))
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	dispatcher := buildB8Dispatcher(t, &twoQuestionChildModel{
		defaults: &chat.Options{Model: "b8-two-questions"},
	}, policy, staticHookResolver{bound: bound})

	handle, err := dispatcher.StartTurn(t.Context(), turn.StartTurnRequest{
		SessionID:      "sess-b8-two-questions",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []string{"question"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := dispatcher.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	interruptCount := 0
	endReason := execution.OutcomeError
	for event := range events {
		switch event := event.(type) {
		case runs.TurnInterrupted:
			interruptCount++
			if len(event.Interrupts) != 1 || event.Interrupts[0].Kind != runs.QuestionInterruptKind {
				t.Fatalf("interrupt %d = %#v", interruptCount, event.Interrupts)
			}
			if err := dispatcher.Resume(t.Context(), handle, interrupts.Resolution{
				Approved: true,
				Answer: map[string][]string{
					runs.QuestionFieldID(0): {"answer"},
				},
			}, []string{"question"}); err != nil {
				t.Fatalf("Resume %d: %v", interruptCount, err)
			}
		case runs.TurnEnd:
			endReason = event.Reason
		}
	}
	if interruptCount != 2 {
		t.Fatalf("interrupt count = %d, want 2", interruptCount)
	}
	if endReason != execution.OutcomeCompleted {
		t.Fatalf("turn end = %q, want completed", endReason)
	}
	if got := recorder.count(hooks.PreToolUse, "ask_user"); got != 2 {
		t.Fatalf("PreToolUse(ask_user) = %d, want once for each of two logical calls", got)
	}
	if got := recorder.count(hooks.PostToolUse, "ask_user"); got != 2 {
		t.Fatalf("PostToolUse(ask_user) = %d, want once for each of two logical calls", got)
	}
}

func TestRestartRestoresParkedChildWithoutReplayingPreHook(t *testing.T) {
	const buildID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cwd := t.TempDir()
	store := storetest.NewMemoryProcessStore()
	historyStore := history.NewInMemoryStore()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-restart"},
		childTool:      "shell",
		childArguments: `{"command":"echo original"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)

	firstHooks := &hookCommandRecorder{
		rewriteTool: "shell", rewriteArguments: `{"command":"echo first-hook"}`,
	}
	first := buildB8PersistentDispatcher(t, model, policy, staticHookResolver{
		bound: hooks.NewBound([]hooks.Hook{
			{Event: hooks.PreToolUse, Command: "record", Source: "test"},
			{Event: hooks.PostToolUse, Command: "record", Source: "test"},
		}, hooks.NewRunner(firstHooks, nil)),
	}, store, historyStore, buildID)

	original, err := first.StartTurn(t.Context(), turn.StartTurnRequest{
		SessionID:      "sess-b8-restart",
		Message:        "delegate this work",
		Cwd:            cwd,
		InterruptKinds: []string{"approval"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := first.Events(t.Context(), original)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	sawInterrupt := false
	for event := range events {
		if interrupted, ok := event.(runs.TurnInterrupted); ok {
			sawInterrupt = true
			if len(interrupted.Interrupts) != 1 {
				t.Fatalf("interrupts = %#v", interrupted.Interrupts)
			}
			break
		}
	}
	if !sawInterrupt {
		t.Fatal("original engine did not park on child approval")
	}
	processID, err := first.ProcessID(t.Context(), original)
	if err != nil {
		t.Fatalf("ProcessID: %v", err)
	}
	if got := firstHooks.count(hooks.PreToolUse, "shell"); got != 1 {
		t.Fatalf("first PreToolUse(shell) = %d, want 1", got)
	}

	restoredHooks := &hookCommandRecorder{
		rewriteTool: "shell", rewriteArguments: `{"command":"echo must-not-run"}`,
	}
	restored := buildB8PersistentDispatcher(t, model, policy, staticHookResolver{
		bound: hooks.NewBound([]hooks.Hook{
			{Event: hooks.PreToolUse, Command: "record", Source: "test"},
			{Event: hooks.PostToolUse, Command: "record", Source: "test"},
		}, hooks.NewRunner(restoredHooks, nil)),
	}, store, historyStore, buildID)
	restoredHandle, err := restored.Rehydrate(t.Context(), turn.RehydrateRequest{
		SessionID: original.SessionID,
		TurnID:    original.TurnID,
		ProcessID: processID,
		Cwd:       cwd,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	restoredEvents, err := restored.Events(t.Context(), restoredHandle)
	if err != nil {
		t.Fatalf("restored Events: %v", err)
	}
	const humanArguments = `{"command":"echo human-after-restart"}`
	if err := restored.Resume(t.Context(), restoredHandle, interrupts.Resolution{
		Approved: true, Arguments: humanArguments,
	}, []string{"approval"}); err != nil {
		t.Fatalf("restored Resume: %v", err)
	}

	var (
		childArguments string
		endReason      execution.Outcome
	)
	for event := range restoredEvents {
		switch event := event.(type) {
		case runs.ToolCallStart:
			if event.ToolName == "shell" {
				childArguments = event.Arguments
			}
		case runs.TurnEnd:
			endReason = event.Reason
		}
	}
	if endReason != execution.OutcomeCompleted {
		t.Fatalf("restored turn end = %q, want completed", endReason)
	}
	if childArguments != humanArguments {
		t.Fatalf("restored child arguments = %q, want %q", childArguments, humanArguments)
	}
	if got := restoredHooks.count(hooks.PreToolUse, "shell"); got != 0 {
		t.Fatalf("restored PreToolUse(shell) = %d, want 0 (durable gate plan must be reused)", got)
	}
	if got := restoredHooks.count(hooks.PostToolUse, "shell"); got != 1 {
		t.Fatalf("restored PostToolUse(shell) = %d, want 1", got)
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list process snapshots: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("terminal restored tree leaked snapshots: %v", ids)
	}
}

func TestCancelParkedChildCleansWholeProcessTree(t *testing.T) {
	const buildID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	store := storetest.NewMemoryProcessStore()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-cancel"},
		childTool:      "shell",
		childArguments: `{"command":"echo must-not-run"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	dispatcher := buildB8PersistentDispatcher(
		t, model, policy, staticHookResolver{}, store, history.NewInMemoryStore(), buildID,
	)
	handle, err := dispatcher.StartTurn(t.Context(), turn.StartTurnRequest{
		SessionID:      "sess-b8-child-cancel",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []string{"approval"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := dispatcher.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	interruptsSeen := 0
	terminalCount := 0
	endReason := execution.OutcomeError
	for event := range events {
		switch event := event.(type) {
		case runs.TurnInterrupted:
			interruptsSeen++
			if err := dispatcher.Cancel(t.Context(), handle); err != nil {
				t.Fatalf("Cancel: %v", err)
			}
		case runs.TurnEnd:
			terminalCount++
			endReason = event.Reason
		}
	}
	if interruptsSeen != 1 || terminalCount != 1 || endReason != execution.OutcomeCanceled {
		t.Fatalf("interrupts/terminals/reason = %d/%d/%q", interruptsSeen, terminalCount, endReason)
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("cancel parked child leaked snapshots: %v", ids)
	}
}

func TestRehydrateRejectsMissingChildSnapshot(t *testing.T) {
	const buildID = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	store := storetest.NewMemoryProcessStore()
	historyStore := history.NewInMemoryStore()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-missing"},
		childTool:      "shell",
		childArguments: `{"command":"echo original"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	first := buildB8PersistentDispatcher(
		t, model, policy, staticHookResolver{}, store, historyStore, buildID,
	)
	handle, err := first.StartTurn(t.Context(), turn.StartTurnRequest{
		SessionID:      "sess-b8-child-missing",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []string{"approval"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := first.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	for event := range events {
		if _, ok := event.(runs.TurnInterrupted); ok {
			break
		}
	}
	rootID, err := first.ProcessID(t.Context(), handle)
	if err != nil {
		t.Fatalf("ProcessID: %v", err)
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	var childID string
	for _, id := range ids {
		if id != rootID {
			childID = id
			break
		}
	}
	if childID == "" {
		t.Fatalf("parked tree snapshots = %v, want root + child", ids)
	}
	if err := store.Apply(t.Context(), core.ProcessSnapshotChange{DeleteRoots: []string{childID}}); err != nil {
		t.Fatalf("delete child snapshot: %v", err)
	}

	restored := buildB8PersistentDispatcher(
		t, model, policy, staticHookResolver{}, store, historyStore, buildID,
	)
	_, err = restored.Rehydrate(t.Context(), turn.RehydrateRequest{
		SessionID: handle.SessionID,
		TurnID:    handle.TurnID,
		ProcessID: rootID,
		Cwd:       t.TempDir(),
	})
	if !errors.Is(err, agentexec.ErrProcessSnapshotLost) {
		t.Fatalf("Rehydrate error = %v, want process snapshot lost", err)
	}
}

func TestChildApproveCancelRaceHasOneTerminal(t *testing.T) {
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-race"},
		childTool:      "shell",
		childArguments: `{"command":"echo race"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	dispatcher := buildB8Dispatcher(t, model, policy, staticHookResolver{})

	for index := range 20 {
		handle, err := dispatcher.StartTurn(t.Context(), turn.StartTurnRequest{
			SessionID:      "sess-b8-race-" + string(rune('a'+index)),
			Message:        "delegate this work",
			Cwd:            t.TempDir(),
			InterruptKinds: []string{"approval"},
		})
		if err != nil {
			t.Fatalf("iteration %d StartTurn: %v", index, err)
		}
		events, err := dispatcher.Events(t.Context(), handle)
		if err != nil {
			t.Fatalf("iteration %d Events: %v", index, err)
		}

		terminalCount := 0
		successfulChildEnds := 0
		raced := false
		for event := range events {
			switch event := event.(type) {
			case runs.TurnInterrupted:
				raced = true
				start := make(chan struct{})
				var (
					wg        sync.WaitGroup
					resumeErr error
					cancelErr error
				)
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					resumeErr = dispatcher.Resume(t.Context(), handle, interrupts.Resolution{Approved: true}, []string{"approval"})
				}()
				go func() {
					defer wg.Done()
					<-start
					cancelErr = dispatcher.Cancel(t.Context(), handle)
				}()
				close(start)
				wg.Wait()
				if resumeErr != nil && !errors.Is(resumeErr, turn.ErrParkClaimed) && !errors.Is(resumeErr, turn.ErrTurnNotFound) {
					t.Fatalf("iteration %d Resume race error = %v", index, resumeErr)
				}
				if cancelErr != nil && !errors.Is(cancelErr, turn.ErrTurnNotFound) {
					t.Fatalf("iteration %d Cancel race error = %v", index, cancelErr)
				}
				if resumeErr != nil && cancelErr != nil {
					t.Fatalf("iteration %d both racers lost: resume=%v cancel=%v", index, resumeErr, cancelErr)
				}
			case runs.ToolCallEnd:
				if !event.Denied && event.Err == "" {
					successfulChildEnds++
				}
			case runs.TurnEnd:
				terminalCount++
			}
		}
		if !raced || terminalCount != 1 {
			t.Fatalf("iteration %d raced/terminals = %v/%d", index, raced, terminalCount)
		}
		if successfulChildEnds > 2 {
			// At most task + one child tool can complete; a larger number means
			// the pending child call was replayed.
			t.Fatalf("iteration %d successful tool ends = %d, want <= 2", index, successfulChildEnds)
		}
	}
}

func buildB8Dispatcher(
	t *testing.T,
	model chat.Model,
	policy interface {
		turn.ApprovalGate
		SetMode(context.Context, approval.Mode) error
	},
	hookResolver staticHookResolver,
) turnDriver {
	t.Helper()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{
		Workdir:   t.TempDir(),
		Approval:  policy,
		Interrupt: suspension.Interrupt,
	})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupToolEnvironment(t, built)
	engine, err := agentexec.New(t.Context(), agentexec.Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
	})
	if err != nil {
		t.Fatalf("agentexec.New: %v", err)
	}
	dispatcher, err := turn.New(turnDeps(engine, withApproval(policy), func(deps *turn.Dependencies) {
		deps.Hooks = hookResolver
	}))
	if err != nil {
		t.Fatalf("turn.New: %v", err)
	}
	t.Cleanup(func() { _ = dispatcher.Close() })
	return dispatcher
}

func buildB8PersistentDispatcher(
	t *testing.T,
	model chat.Model,
	policy interface {
		turn.ApprovalGate
		SetMode(context.Context, approval.Mode) error
	},
	hookResolver staticHookResolver,
	store core.ProcessStore,
	historyStore history.Store,
	buildID string,
) turnDriver {
	t.Helper()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{
		Workdir:   t.TempDir(),
		Approval:  policy,
		Interrupt: suspension.Interrupt,
	})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupToolEnvironment(t, built)
	engine, err := agentexec.New(t.Context(), agentexec.Config{
		BuildID:      buildID,
		ChatClient:   client,
		HistoryStore: historyStore,
		ProcessStore: store,
		ToolResolver: built.Resolver,
	})
	if err != nil {
		t.Fatalf("agentexec.New: %v", err)
	}
	dispatcher, err := turn.New(turnDeps(engine, withApproval(policy), func(deps *turn.Dependencies) {
		deps.Hooks = hookResolver
	}))
	if err != nil {
		t.Fatalf("turn.New: %v", err)
	}
	t.Cleanup(func() {
		_ = dispatcher.Close()
	})
	return dispatcher
}

type childToolModel struct {
	defaults       *chat.Options
	childTool      string
	childArguments string
}

type twoQuestionChildModel struct {
	defaults *chat.Options
}

func (m *twoQuestionChildModel) DefaultOptions() chat.Options { return *m.defaults }

func (m *twoQuestionChildModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolCallNamed(request.Messages, "task"):
		return makeText("root complete")
	case countToolCalls(request.Messages, "ask_user") >= 2:
		return makeText("child complete")
	case countToolCalls(request.Messages, "ask_user") == 1:
		return makeToolCall("ask_user", `{"questions":[{"question":"Second question?"}]}`)
	case userMentions(request.Messages, "delegate"):
		return makeToolCall("task", `{"prompt":"perform the child work"}`)
	default:
		return makeToolCall("ask_user", `{"questions":[{"question":"First question?"}]}`)
	}
}

func (m *twoQuestionChildModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func (m *childToolModel) DefaultOptions() chat.Options { return *m.defaults }

func (m *childToolModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolCallNamed(request.Messages, "task"):
		return makeText("root complete")
	case hasToolCallNamed(request.Messages, m.childTool):
		return makeText("child complete")
	case userMentions(request.Messages, "delegate"):
		return makeToolCall("task", `{"prompt":"perform the child work"}`)
	default:
		return makeToolCall(m.childTool, m.childArguments)
	}
}

func (m *childToolModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func hasToolCallNamed(messages []chat.Message, name string) bool {
	for _, message := range messages {
		if message.Role != chat.RoleAssistant {
			continue
		}
		for _, part := range message.Parts {
			if part.Kind == chat.PartToolCall && part.ToolCall != nil && part.ToolCall.Name == name {
				return true
			}
		}
	}
	return false
}

func countToolCalls(messages []chat.Message, name string) int {
	count := 0
	for _, message := range messages {
		if message.Role != chat.RoleAssistant {
			continue
		}
		for _, part := range message.Parts {
			if part.Kind == chat.PartToolCall && part.ToolCall != nil && part.ToolCall.Name == name {
				count++
			}
		}
	}
	return count
}

func userMentions(messages []chat.Message, text string) bool {
	for _, message := range messages {
		if message.Role == chat.RoleUser && strings.Contains(message.Text(), text) {
			return true
		}
	}
	return false
}

type staticHookResolver struct {
	bound *hooks.Bound
	err   error
}

func (r staticHookResolver) For(context.Context, string) (*hooks.Bound, error) {
	return r.bound, r.err
}

type hookCommandRecorder struct {
	mu               sync.Mutex
	inputs           []hooks.Input
	rewriteTool      string
	rewriteArguments string
}

func (r *hookCommandRecorder) RunHookCommand(_ context.Context, request hooks.CommandRequest) hooks.CommandResult {
	input := request.Input
	r.mu.Lock()
	r.inputs = append(r.inputs, input)
	r.mu.Unlock()
	if input.Event == hooks.PreToolUse && input.Tool != nil &&
		input.Tool.Name == r.rewriteTool && r.rewriteArguments != "" {
		return hooks.CommandResult{Decision: hooks.CommandDecision{RewriteArguments: r.rewriteArguments}}
	}
	return hooks.CommandResult{}
}

func (r *hookCommandRecorder) count(event hooks.Event, toolName string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, input := range r.inputs {
		if input.Event == event && input.Tool != nil && input.Tool.Name == toolName {
			count++
		}
	}
	return count
}
