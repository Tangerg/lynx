package turn_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/inmemory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestChildToolsShareRootHITLAndHookContract(t *testing.T) {
	tests := []childHITLScenario{
		{
			name:             "approval with human edited arguments",
			childTool:        "shell",
			childArguments:   `{"command":"echo original","description":"Print original"}`,
			interruptKinds:   []execution.InterruptKind{execution.ApprovalInterrupt},
			wantInterrupt:    new(execution.ApprovalInterrupt),
			resolution:       interrupts.Resolution{Approved: true, Arguments: `{"command":"echo human","description":"Print human"}`},
			rewriteArguments: `{"command":"echo hook","description":"Print hook"}`,
		},
		{
			name:             "approval denial",
			childTool:        "shell",
			childArguments:   `{"command":"echo original","description":"Print original"}`,
			interruptKinds:   []execution.InterruptKind{execution.ApprovalInterrupt},
			wantInterrupt:    new(execution.ApprovalInterrupt),
			resolution:       interrupts.Resolution{Approved: false, Reason: "not this time"},
			rewriteArguments: `{"command":"echo hook","description":"Print hook"}`,
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
			interruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
			wantInterrupt:  new(execution.QuestionInterrupt),
			resolution: interrupts.Resolution{
				Approved: true,
				Answers:  [][]string{{"Yes"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, recorder := runChildHITLScenario(t, test)
			assertChildHITLOutcome(t, test, outcome, recorder)
		})
	}
}

type childHITLScenario struct {
	name             string
	childTool        string
	childArguments   string
	interruptKinds   []execution.InterruptKind
	wantInterrupt    *execution.InterruptKind
	resolution       interrupts.Resolution
	rewriteArguments string
}

type childHITLOutcome struct {
	interruptCount int
	childEvents    int
	endReason      execution.Outcome
}

func runChildHITLScenario(t *testing.T, scenario childHITLScenario) (childHITLOutcome, *hookCommandRecorder) {
	t.Helper()
	recorder := &hookCommandRecorder{rewriteTool: scenario.childTool, rewriteArguments: scenario.rewriteArguments}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.PreToolUse, Command: "record", Source: "test"},
		{Event: hooks.PostToolUse, Command: "record", Source: "test"},
	}, hooks.NewRunner(recorder, nil))
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	controller := buildB8Controller(t, &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-hitl"},
		childTool:      scenario.childTool,
		childArguments: scenario.childArguments,
	}, policy, staticHookResolver{bound: bound})

	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-" + strings.ReplaceAll(scenario.name, " ", "-"),
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: scenario.interruptKinds,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	var outcome childHITLOutcome
	for event := range events {
		switch event := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, event)
			outcome.interruptCount++
			if len(event.Suspensions) != 1 {
				t.Fatalf("suspensions = %#v", event.Suspensions)
			}
			pending := event.Suspensions[0].Interrupt
			if scenario.wantInterrupt == nil || pending.Kind != *scenario.wantInterrupt {
				t.Fatalf("interrupt kind = %q, want %q", pending.Kind, scenario.wantInterrupt)
			}
			toolName, _ := pending.Tool()
			if toolName != scenario.childTool {
				t.Fatalf("interrupt tool = %q, want child %q (task must not be gated)", toolName, scenario.childTool)
			}
			if err := controller.Resume(
				t.Context(),
				handle,
				answersForBarrier(event, scenario.resolution),
				scenario.interruptKinds,
			); err != nil {
				t.Fatalf("Resume: %v", err)
			}
		case runs.ToolCallStart:
			if event.ToolName == scenario.childTool {
				outcome.childEvents++
			}
		case runs.TurnEnd:
			outcome.endReason = event.Reason
		}
	}
	return outcome, recorder
}

func assertChildHITLOutcome(t *testing.T, scenario childHITLScenario, outcome childHITLOutcome, recorder *hookCommandRecorder) {
	t.Helper()
	if scenario.wantInterrupt == nil && outcome.interruptCount != 0 {
		t.Fatalf("safe child interrupt count = %d, want 0", outcome.interruptCount)
	}
	if scenario.wantInterrupt != nil && outcome.interruptCount != 1 {
		t.Fatalf("interrupt count = %d, want 1", outcome.interruptCount)
	}
	if outcome.endReason != execution.OutcomeCompleted {
		t.Fatalf("turn end = %q, want completed", outcome.endReason)
	}
	if outcome.childEvents != 0 {
		t.Fatalf("child tool events leaked into root turn: %d", outcome.childEvents)
	}
	if got := recorder.count(hooks.PreToolUse, scenario.childTool); got != 1 {
		t.Fatalf("PreToolUse(%s) count = %d, want 1", scenario.childTool, got)
	}
	if got := recorder.count(hooks.PostToolUse, scenario.childTool); got != 1 {
		t.Fatalf("PostToolUse(%s) count = %d, want 1", scenario.childTool, got)
	}
	if got := recorder.count(hooks.PreToolUse, "delegate_task"); got != 0 {
		t.Fatalf("PreToolUse(task) count = %d, want 0", got)
	}
	if got := recorder.count(hooks.PostToolUse, "delegate_task"); got != 0 {
		t.Fatalf("PostToolUse(task) count = %d, want 0", got)
	}
}

func TestChildCanSuspendTwiceOnTheSameRun(t *testing.T) {
	recorder := &hookCommandRecorder{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.PreToolUse, Command: "record", Source: "test"},
		{Event: hooks.PostToolUse, Command: "record", Source: "test"},
	}, hooks.NewRunner(recorder, nil))
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	controller := buildB8Controller(t, &twoQuestionChildModel{
		defaults: &chat.Options{Model: "b8-two-questions"},
	}, policy, staticHookResolver{bound: bound})

	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-two-questions",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	interruptCount := 0
	endReason := execution.OutcomeError
	for event := range events {
		switch event := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, event)
			interruptCount++
			if len(event.Suspensions) != 1 ||
				event.Suspensions[0].Interrupt.Kind != execution.QuestionInterrupt {
				t.Fatalf("interrupt %d = %#v", interruptCount, event.Suspensions)
			}
			resolution := interrupts.Resolution{
				Approved: true,
				Answers:  [][]string{{"answer"}},
			}
			if err := controller.Resume(
				t.Context(),
				handle,
				answersForBarrier(event, resolution),
				[]execution.InterruptKind{execution.QuestionInterrupt},
			); err != nil {
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

func TestCompleteAnswerSetDrivesParallelChildSuspensionsWithoutSecondBarrier(t *testing.T) {
	controller := buildB8Controller(
		t,
		&parallelQuestionChildModel{defaults: &chat.Options{Model: "b8-parallel-questions"}},
		mustApprovalPolicy(t, approval.ModeBalanced, nil),
		staticHookResolver{},
	)
	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-parallel-questions",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	interruptCount := 0
	endReason := execution.OutcomeError
	for event := range events {
		switch event := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, event)
			interruptCount++
			if len(event.Suspensions) != 2 {
				t.Fatalf("barrier suspensions = %#v, want two", event.Suspensions)
			}
			answers := make([]agentexec.SuspensionAnswer, len(event.Suspensions))
			for index, boundary := range event.Suspensions {
				answers[index] = agentexec.SuspensionAnswer{
					ProcessID:    boundary.ProcessID,
					SuspensionID: boundary.SuspensionID,
					Resolution: interrupts.Resolution{
						Approved: true,
						Answers:  [][]string{{fmt.Sprintf("answer-%d", index+1)}},
					},
				}
			}
			if err := controller.Resume(
				t.Context(),
				handle,
				answers,
				[]execution.InterruptKind{execution.QuestionInterrupt},
			); err != nil {
				t.Fatalf("Resume complete answer set: %v", err)
			}
		case runs.TurnEnd:
			endReason = event.Reason
		}
	}
	if interruptCount != 1 {
		t.Fatalf("TreeInterrupted count = %d, want one atomic application barrier", interruptCount)
	}
	if endReason != execution.OutcomeCompleted {
		t.Fatalf("turn end = %q, want completed", endReason)
	}
}

func TestRestartResumesCompleteSiblingAnswerSetWithoutReplayingBarrier(t *testing.T) {
	const buildID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	cwd := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "runtime.db")
	firstDatabase, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	t.Cleanup(func() { _ = firstDatabase.Close() })
	sess, err := sqlite.NewSessionStore(firstDatabase).Create(
		t.Context(),
		"restart sibling answer set",
		cwd,
	)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	model := &parallelQuestionChildModel{
		defaults: &chat.Options{Model: "b8-restart-parallel-questions"},
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	firstCheckpoints := sqlite.NewExecutorCheckpointStore(firstDatabase)
	first := buildB8PersistentController(
		t,
		model,
		policy,
		staticHookResolver{},
		firstCheckpoints,
		sqlite.NewMessageStore(firstDatabase),
		buildID,
	)
	original, err := first.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      sess.ID,
		Message:        "delegate this work",
		Cwd:            cwd,
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := first.Events(t.Context(), original)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var barrier runs.TreeInterrupted
	for event := range events {
		if interrupted, ok := event.Payload.(runs.TreeInterrupted); ok {
			barrier = interrupted
			persistTreeBarrier(t, barrier, firstCheckpoints)
			break
		}
	}
	if len(barrier.Suspensions) != 2 {
		t.Fatalf("original barrier = %#v, want two sibling suspensions", barrier.Suspensions)
	}
	processID, err := first.ProcessID(t.Context(), original)
	if err != nil {
		t.Fatalf("ProcessID: %v", err)
	}

	restoredDatabase, err := sqlite.Open(databasePath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = restoredDatabase.Close() })
	restored := buildB8PersistentController(
		t,
		model,
		policy,
		staticHookResolver{},
		sqlite.NewExecutorCheckpointStore(restoredDatabase),
		sqlite.NewMessageStore(restoredDatabase),
		buildID,
	)
	restoredHandle, err := restored.Rehydrate(t.Context(), runs.RehydrateTurn{
		SessionID: original.SessionID,
		TurnID:    original.TurnID,
		ProcessID: processID,
		RootRunID: "run-root",
		Cwd:       cwd,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	restoredEvents, err := restored.Events(t.Context(), restoredHandle)
	if err != nil {
		t.Fatalf("restored Events: %v", err)
	}
	answers := make([]agentexec.SuspensionAnswer, len(barrier.Suspensions))
	for index, boundary := range barrier.Suspensions {
		answers[index] = agentexec.SuspensionAnswer{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Resolution: interrupts.Resolution{
				Approved: true,
				Answers:  [][]string{{fmt.Sprintf("restored-answer-%d", index+1)}},
			},
		}
	}
	if err := restored.Resume(
		t.Context(),
		restoredHandle,
		answers,
		[]execution.InterruptKind{execution.QuestionInterrupt},
	); err != nil {
		t.Fatalf("restored Resume: %v", err)
	}
	replayedBarriers := 0
	endReason := execution.OutcomeError
	for event := range restoredEvents {
		switch payload := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, payload)
			replayedBarriers++
		case runs.TurnEnd:
			endReason = payload.Reason
		}
	}
	if replayedBarriers != 0 {
		t.Fatalf("restored continuation published %d intermediate barriers, want none", replayedBarriers)
	}
	if endReason != execution.OutcomeCompleted {
		t.Fatalf("restored turn end = %q, want completed", endReason)
	}
}

func TestCompleteAnswerSetDoesNotPersistDuringLiveContinuation(t *testing.T) {
	const buildID = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	baseStore := newMemoryCheckpointStore()
	store := &failNthSaveCheckpointStore{
		testCheckpointStore: baseStore,
		failAt:              2,
		err:                 errors.New("unexpected executor-owned checkpoint write"),
	}
	controller := buildB8PersistentController(
		t,
		&parallelQuestionChildModel{
			defaults: &chat.Options{Model: "b8-parallel-checkpoint-failure"},
		},
		mustApprovalPolicy(t, approval.ModeBalanced, nil),
		staticHookResolver{},
		store,
		inmemory.New(),
		buildID,
	)
	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-parallel-checkpoint-failure",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	barrierCount := 0
	terminalCount := 0
	for event := range events {
		switch payload := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, payload, store)
			barrierCount++
			if len(payload.Suspensions) != 2 {
				t.Fatalf("barrier suspensions = %#v, want two", payload.Suspensions)
			}
			if err := controller.Resume(
				t.Context(),
				handle,
				questionAnswersForBarrier(payload, "checkpoint-failure"),
				[]execution.InterruptKind{execution.QuestionInterrupt},
			); err != nil {
				t.Fatalf("Resume complete answer set: %v", err)
			}
		case runs.TurnEnd:
			terminalCount++
			if payload.Reason != execution.OutcomeCompleted || payload.Problem != nil {
				t.Fatalf("TurnEnd = %+v, want completed without hidden checkpoint I/O", payload)
			}
		}
	}
	if barrierCount != 1 {
		t.Fatalf("TreeInterrupted count = %d, want original barrier only", barrierCount)
	}
	if terminalCount != 1 {
		t.Fatalf("TurnEnd count = %d, want one", terminalCount)
	}
	if saves := store.saves.Load(); saves != 1 {
		t.Fatalf("checkpoint save attempts = %d, want only the application-committed barrier", saves)
	}
}

func TestCompleteAnswerSetEncodingFailurePrecedesAnyContinuationSideEffect(t *testing.T) {
	const buildID = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	store := &failNthSaveCheckpointStore{
		testCheckpointStore: newMemoryCheckpointStore(),
		failAt:              -1,
	}
	controller := buildB8PersistentController(
		t,
		&parallelQuestionChildModel{
			defaults: &chat.Options{Model: "b8-parallel-encoding-failure"},
		},
		mustApprovalPolicy(t, approval.ModeBalanced, nil),
		staticHookResolver{},
		store,
		inmemory.New(),
		buildID,
	)
	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-parallel-encoding-failure",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	barrierCount := 0
	terminalCount := 0
	for event := range events {
		switch payload := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, payload, store)
			barrierCount++
			answers := questionAnswersForBarrier(payload, "encoding-failure")
			answers[len(answers)-1].Resolution.RememberScope = approval.Scope("unknown")
			err := controller.Resume(
				t.Context(),
				handle,
				answers,
				[]execution.InterruptKind{execution.QuestionInterrupt},
			)
			if err == nil || !strings.Contains(err.Error(), "unknown remember scope") {
				t.Fatalf("Resume encoding error = %v, want invalid remember scope", err)
			}
		case runs.TurnEnd:
			terminalCount++
			if payload.Reason != execution.OutcomeError || payload.Problem == nil {
				t.Fatalf("TurnEnd = %+v, want canonical error terminal", payload)
			}
		}
	}
	if barrierCount != 1 || terminalCount != 1 {
		t.Fatalf(
			"barriers/terminals = %d/%d, want original barrier and one terminal",
			barrierCount,
			terminalCount,
		)
	}
	if saves := store.saves.Load(); saves != 1 {
		t.Fatalf("checkpoint save attempts = %d, want initial barrier only", saves)
	}
}

func TestRestartRestoresParkedChildWithoutReplayingPreHook(t *testing.T) {
	const buildID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cwd := t.TempDir()
	store := newMemoryCheckpointStore()
	historyStore := inmemory.New()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-restart"},
		childTool:      "shell",
		childArguments: `{"command":"echo original","description":"Print original"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)

	firstHooks := &hookCommandRecorder{
		rewriteTool: "shell", rewriteArguments: `{"command":"echo first-hook","description":"Print first hook"}`,
	}
	first := buildB8PersistentController(t, model, policy, staticHookResolver{
		bound: hooks.NewBound([]hooks.Hook{
			{Event: hooks.PreToolUse, Command: "record", Source: "test"},
			{Event: hooks.PostToolUse, Command: "record", Source: "test"},
		}, hooks.NewRunner(firstHooks, nil)),
	}, store, historyStore, buildID)

	original, err := first.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-restart",
		Message:        "delegate this work",
		Cwd:            cwd,
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := first.Events(t.Context(), original)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var barrier runs.TreeInterrupted
	for event := range events {
		if interrupted, ok := event.Payload.(runs.TreeInterrupted); ok {
			barrier = interrupted
			persistTreeBarrier(t, barrier, store)
			if len(interrupted.Suspensions) != 1 {
				t.Fatalf("suspensions = %#v", interrupted.Suspensions)
			}
			break
		}
	}
	if len(barrier.Suspensions) == 0 {
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
		rewriteTool: "shell", rewriteArguments: `{"command":"echo must-not-run","description":"Print forbidden output"}`,
	}
	restored := buildB8PersistentController(t, model, policy, staticHookResolver{
		bound: hooks.NewBound([]hooks.Hook{
			{Event: hooks.PreToolUse, Command: "record", Source: "test"},
			{Event: hooks.PostToolUse, Command: "record", Source: "test"},
			{Event: hooks.SubagentStart, Command: "record", Source: "test"},
			{Event: hooks.SubagentStop, Command: "record", Source: "test"},
		}, hooks.NewRunner(restoredHooks, nil)),
	}, store, historyStore, buildID)
	restoredHandle, err := restored.Rehydrate(t.Context(), runs.RehydrateTurn{
		SessionID: original.SessionID,
		TurnID:    original.TurnID,
		ProcessID: processID,
		RootRunID: "run-root",
		ChildRuns: []runs.ChildRunBinding{{
			ProcessID:   barrier.Suspensions[0].ProcessID,
			RunID:       "run-child",
			ParentRunID: "run-root",
		}},
		Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	restoredEvents, err := restored.Events(t.Context(), restoredHandle)
	if err != nil {
		t.Fatalf("restored Events: %v", err)
	}
	const humanArguments = `{"command":"echo human-after-restart","description":"Print after restart"}`
	if err := restored.Resume(
		t.Context(),
		restoredHandle,
		answersForBarrier(barrier, interrupts.Resolution{Approved: true, Arguments: humanArguments}),
		[]execution.InterruptKind{execution.ApprovalInterrupt},
	); err != nil {
		t.Fatalf("restored Resume: %v", err)
	}

	endReason := execution.OutcomeError
	leakedChildEvents := 0
	for event := range restoredEvents {
		switch event := event.Payload.(type) {
		case runs.ToolCallStart:
			if event.ToolName == "shell" {
				leakedChildEvents++
			}
		case runs.TurnEnd:
			endReason = event.Reason
		}
	}
	if endReason != execution.OutcomeCompleted {
		t.Fatalf("restored turn end = %q, want completed", endReason)
	}
	if leakedChildEvents != 0 {
		t.Fatalf("restored child tool events leaked into root turn: %d", leakedChildEvents)
	}
	if got := restoredHooks.count(hooks.PreToolUse, "shell"); got != 0 {
		t.Fatalf("restored PreToolUse(shell) = %d, want 0 (durable gate plan must be reused)", got)
	}
	if got := restoredHooks.count(hooks.PostToolUse, "shell"); got != 1 {
		t.Fatalf("restored PostToolUse(shell) = %d, want 1", got)
	}
	if got := restoredHooks.inputsFor(hooks.SubagentStart); len(got) != 0 {
		t.Fatalf("restored SubagentStart inputs = %#v, want no replay", got)
	}
	stopInputs := restoredHooks.inputsFor(hooks.SubagentStop)
	if len(stopInputs) != 1 {
		t.Fatalf("restored SubagentStop inputs = %#v, want 1", stopInputs)
	}
	stop := stopInputs[0].Subagent
	if stop == nil ||
		stop.RunID != "run-child" ||
		stop.ParentRunID != "run-root" ||
		stop.Description != "focused child work" ||
		stop.Prompt != "perform the child work" ||
		stop.Status != hooks.SubagentCompleted ||
		stop.Result != "child complete" {
		t.Fatalf("restored SubagentStop = %+v", stop)
	}
	joinTurnCleanup(t, restored, restoredHandle)
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list executor checkpoints: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("terminal executor checkpoints = %v, want one App-owned aggregate pending cleanup", ids)
	}
	if err := store.DeleteCheckpoints(t.Context(), original.SessionID, []string{processID}); err != nil {
		t.Fatalf("application terminal checkpoint cleanup: %v", err)
	}
	if ids, err = store.List(t.Context()); err != nil || len(ids) != 0 {
		t.Fatalf("checkpoint rows after application cleanup = %v, %v", ids, err)
	}
}

func TestCancelParkedChildCleansWholeProcessTree(t *testing.T) {
	const buildID = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	store := newMemoryCheckpointStore()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-cancel"},
		childTool:      "shell",
		childArguments: `{"command":"echo must-not-run","description":"Print forbidden output"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	controller := buildB8PersistentController(
		t, model, policy, staticHookResolver{}, store, inmemory.New(), buildID,
	)
	handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-child-cancel",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	interruptsSeen := 0
	terminalCount := 0
	endReason := execution.OutcomeError
	var processID string
	for event := range events {
		switch event := event.Payload.(type) {
		case runs.TreeInterrupted:
			persistTreeBarrier(t, event, store)
			interruptsSeen++
			processID, err = controller.ProcessID(t.Context(), handle)
			if err != nil {
				t.Fatalf("ProcessID: %v", err)
			}
			if err := controller.Cancel(t.Context(), handle); err != nil {
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
	if err := controller.Cancel(t.Context(), handle); err != nil && !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("join terminal cleanup: %v", err)
	}
	ids, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("canceled executor checkpoints = %v, want one App-owned aggregate pending cleanup", ids)
	}
	if err := store.DeleteCheckpoints(t.Context(), handle.SessionID, []string{processID}); err != nil {
		t.Fatalf("application cancel checkpoint cleanup: %v", err)
	}
	if ids, err = store.List(t.Context()); err != nil || len(ids) != 0 {
		t.Fatalf("checkpoint rows after application cleanup = %v, %v", ids, err)
	}
}

func TestRehydrateRejectsCorruptCheckpointPayload(t *testing.T) {
	const buildID = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	store := newMemoryCheckpointStore()
	historyStore := inmemory.New()
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-missing"},
		childTool:      "shell",
		childArguments: `{"command":"echo original","description":"Print original"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	first := buildB8PersistentController(
		t, model, policy, staticHookResolver{}, store, historyStore, buildID,
	)
	handle, err := first.StartTurn(t.Context(), runs.StartTurn{
		SessionID:      "sess-b8-child-missing",
		Message:        "delegate this work",
		Cwd:            t.TempDir(),
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := first.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	for event := range events {
		if barrier, ok := event.Payload.(runs.TreeInterrupted); ok {
			persistTreeBarrier(t, barrier, store)
			break
		}
	}
	rootID, err := first.ProcessID(t.Context(), handle)
	if err != nil {
		t.Fatalf("ProcessID: %v", err)
	}
	checkpoint, err := store.LoadCheckpoint(t.Context(), rootID)
	if err != nil {
		t.Fatalf("load executor checkpoint: %v", err)
	}
	checkpoint.Payload = []byte("{")
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatalf("corrupt opaque checkpoint payload: %v", err)
	}

	restored := buildB8PersistentController(
		t, model, policy, staticHookResolver{}, store, historyStore, buildID,
	)
	_, err = restored.Rehydrate(t.Context(), runs.RehydrateTurn{
		SessionID: handle.SessionID,
		TurnID:    handle.TurnID,
		ProcessID: rootID,
		RootRunID: "run-root",
		Cwd:       t.TempDir(),
	})
	if !errors.Is(err, agentexec.ErrExecutorCheckpointLost) {
		t.Fatalf("Rehydrate error = %v, want executor checkpoint lost", err)
	}
}

func TestChildApproveCancelRaceHasOneTerminal(t *testing.T) {
	model := &childToolModel{
		defaults:       &chat.Options{Model: "b8-child-race"},
		childTool:      "shell",
		childArguments: `{"command":"echo race","description":"Print race"}`,
	}
	policy := mustApprovalPolicy(t, approval.ModeBalanced, nil)
	controller := buildB8Controller(t, model, policy, staticHookResolver{})

	for index := range 20 {
		handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
			SessionID:      "sess-b8-race-" + string(rune('a'+index)),
			Message:        "delegate this work",
			Cwd:            t.TempDir(),
			InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
		})
		if err != nil {
			t.Fatalf("iteration %d StartTurn: %v", index, err)
		}
		events, err := controller.Events(t.Context(), handle)
		if err != nil {
			t.Fatalf("iteration %d Events: %v", index, err)
		}

		terminalCount := 0
		leakedChildEvents := 0
		raced := false
		for event := range events {
			switch event := event.Payload.(type) {
			case runs.TreeInterrupted:
				persistTreeBarrier(t, event)
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
					resumeErr = controller.Resume(
						t.Context(),
						handle,
						answersForBarrier(event, interrupts.Resolution{Approved: true}),
						[]execution.InterruptKind{execution.ApprovalInterrupt},
					)
				}()
				go func() {
					defer wg.Done()
					<-start
					cancelErr = controller.Cancel(t.Context(), handle)
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
			case runs.ToolCallStart:
				if event.ToolName == "shell" {
					leakedChildEvents++
				}
			case runs.TurnEnd:
				terminalCount++
			}
		}
		if !raced || terminalCount != 1 {
			t.Fatalf("iteration %d raced/terminals = %v/%d", index, raced, terminalCount)
		}
		if leakedChildEvents != 0 {
			t.Fatalf("iteration %d leaked child tool events = %d, want 0", index, leakedChildEvents)
		}
	}
}

func TestCompleteSiblingAnswerSetCancelRaceHasOneTerminalAndNoSecondBarrier(t *testing.T) {
	controller := buildB8Controller(
		t,
		&parallelQuestionChildModel{
			defaults: &chat.Options{Model: "b8-parallel-answer-cancel-race"},
		},
		mustApprovalPolicy(t, approval.ModeBalanced, nil),
		staticHookResolver{},
	)

	for index := range 20 {
		handle, err := controller.StartTurn(t.Context(), runs.StartTurn{
			SessionID:      fmt.Sprintf("sess-b8-parallel-race-%d", index),
			Message:        "delegate this work",
			Cwd:            t.TempDir(),
			InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
		})
		if err != nil {
			t.Fatalf("iteration %d StartTurn: %v", index, err)
		}
		events, err := controller.Events(t.Context(), handle)
		if err != nil {
			t.Fatalf("iteration %d Events: %v", index, err)
		}

		barrierCount := 0
		terminalCount := 0
		for event := range events {
			switch payload := event.Payload.(type) {
			case runs.TreeInterrupted:
				barrierCount++
				if barrierCount != 1 {
					continue
				}
				persistTreeBarrier(t, payload)
				answers := questionAnswersForBarrier(payload, fmt.Sprintf("race-%d", index))
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
					resumeErr = controller.Resume(
						t.Context(),
						handle,
						answers,
						[]execution.InterruptKind{execution.QuestionInterrupt},
					)
				}()
				go func() {
					defer wg.Done()
					<-start
					cancelErr = controller.Cancel(t.Context(), handle)
				}()
				close(start)
				wg.Wait()
				if resumeErr != nil &&
					!errors.Is(resumeErr, turn.ErrParkClaimed) &&
					!errors.Is(resumeErr, turn.ErrTurnNotFound) {
					t.Fatalf("iteration %d Resume race error = %v", index, resumeErr)
				}
				if cancelErr != nil && !errors.Is(cancelErr, turn.ErrTurnNotFound) {
					t.Fatalf("iteration %d Cancel race error = %v", index, cancelErr)
				}
				if resumeErr != nil && cancelErr != nil {
					t.Fatalf(
						"iteration %d both racers lost: resume=%v cancel=%v",
						index,
						resumeErr,
						cancelErr,
					)
				}
			case runs.TurnEnd:
				terminalCount++
				if payload.Reason != execution.OutcomeCompleted &&
					payload.Reason != execution.OutcomeCanceled {
					t.Fatalf("iteration %d TurnEnd reason = %q", index, payload.Reason)
				}
			}
		}
		if barrierCount != 1 || terminalCount != 1 {
			t.Fatalf(
				"iteration %d barriers/terminals = %d/%d, want 1/1",
				index,
				barrierCount,
				terminalCount,
			)
		}
	}
}

func buildB8Controller(
	t *testing.T,
	model chat.Model,
	policy interface {
		turn.ApprovalGate
	},
	hookResolver staticHookResolver,
) turnDriver {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{
		Workdir:   t.TempDir(),
		Interrupt: suspension.Interrupt,
	})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupToolEnvironment(t, built)
	engine, err := agentexec.New(t.Context(), agentexec.Config{
		BuildID:      testProcessBuildID,
		ChatClient:   client,
		Checkpoints:  newMemoryCheckpointStore(),
		ToolResolver: built.Resolver,
	})
	if err != nil {
		t.Fatalf("agentexec.New: %v", err)
	}
	controller, err := turn.New(turnDeps(engine, withApproval(policy), func(deps *turn.Dependencies) {
		deps.Hooks = hookResolver
	}))
	if err != nil {
		t.Fatalf("turn.New: %v", err)
	}
	t.Cleanup(func() { shutdownController(t, controller) })
	return controller
}

func buildB8PersistentController(
	t *testing.T,
	model chat.Model,
	policy interface {
		turn.ApprovalGate
	},
	hookResolver staticHookResolver,
	store testCheckpointStore,
	historyStore history.Store,
	buildID string,
) turnDriver {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	built, err := toolset.Build(t.Context(), toolset.BuildConfig{
		Workdir:   t.TempDir(),
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
		Checkpoints:  store,
		ToolResolver: built.Resolver,
	})
	if err != nil {
		t.Fatalf("agentexec.New: %v", err)
	}
	controller, err := turn.New(turnDeps(engine, withApproval(policy), func(deps *turn.Dependencies) {
		deps.Hooks = hookResolver
	}))
	if err != nil {
		t.Fatalf("turn.New: %v", err)
	}
	t.Cleanup(func() {
		shutdownController(t, controller)
	})
	return controller
}

type childToolModel struct {
	defaults       *chat.Options
	childTool      string
	childArguments string
}

type twoQuestionChildModel struct {
	defaults *chat.Options
}

type parallelQuestionChildModel struct {
	defaults *chat.Options
}

func (m *twoQuestionChildModel) DefaultOptions() chat.Options { return *m.defaults }

func (m *twoQuestionChildModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolCallNamed(request.Messages, "delegate_task"):
		return makeText("root complete")
	case countToolCalls(request.Messages, "ask_user") >= 2:
		return makeText("child complete")
	case countToolCalls(request.Messages, "ask_user") == 1:
		return makeToolCall("ask_user", `{"questions":[{"question":"Second question?"}]}`)
	case userMentions(request.Messages, "delegate"):
		return makeToolCall("delegate_task", `{"summary":"delegated work","instructions":"perform the child work"}`)
	default:
		return makeToolCall("ask_user", `{"questions":[{"question":"First question?"}]}`)
	}
}

func (m *twoQuestionChildModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func (m *parallelQuestionChildModel) DefaultOptions() chat.Options { return *m.defaults }

func (m *parallelQuestionChildModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolCallNamed(request.Messages, "delegate_task"):
		return makeText("root complete")
	case hasToolCallNamed(request.Messages, "ask_user"):
		return makeText("child complete")
	case userMentions(request.Messages, "delegate"):
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{
				ID: "task_1", Name: "delegate_task",
				Arguments: `{"summary":"delegated work","instructions":"perform first child work"}`,
			}),
			chat.NewToolCallPart(chat.ToolCall{
				ID: "task_2", Name: "delegate_task",
				Arguments: `{"summary":"delegated work","instructions":"perform second child work"}`,
			}),
		)
		return chat.NewResponse(chat.Choice{
			Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
		})
	default:
		return makeToolCall("ask_user", `{"questions":[{"question":"Child question?"}]}`)
	}
}

func (m *parallelQuestionChildModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	response, err := m.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func (m *childToolModel) DefaultOptions() chat.Options { return *m.defaults }

func (m *childToolModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolCallNamed(request.Messages, "delegate_task"):
		return makeText("root complete")
	case hasToolCallNamed(request.Messages, m.childTool):
		return makeText("child complete")
	case userMentions(request.Messages, "delegate"):
		return makeToolCall("delegate_task", `{"summary":"focused child work","instructions":"perform the child work"}`)
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

func questionAnswersForBarrier(
	barrier runs.TreeInterrupted,
	prefix string,
) []agentexec.SuspensionAnswer {
	answers := make([]agentexec.SuspensionAnswer, len(barrier.Suspensions))
	for index, boundary := range barrier.Suspensions {
		answers[index] = agentexec.SuspensionAnswer{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Resolution: interrupts.Resolution{
				Approved: true,
				Answers:  [][]string{{fmt.Sprintf("%s-%d", prefix, index+1)}},
			},
		}
	}
	return answers
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

func (r *hookCommandRecorder) inputsFor(event hooks.Event) []hooks.Input {
	r.mu.Lock()
	defer r.mu.Unlock()
	var inputs []hooks.Input
	for _, input := range r.inputs {
		if input.Event == event {
			inputs = append(inputs, input)
		}
	}
	return inputs
}
