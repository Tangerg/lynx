package agentexec

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/chatclient"
	toolcontract "github.com/Tangerg/lynx/tool"
)

func TestInteractionExecutorRestoresWaitingDelegateChildWithoutReadmission(t *testing.T) {
	model := newWaitingDelegateModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask the user for the value required by delegated work.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, err := interactioninput.Require(ctx, "delegate.required-value", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value should the delegate use?"}},
			},
		})
		if err != nil {
			return "", err
		}
		if len(resolution.Answers) != 1 || len(resolution.Answers[0]) != 1 {
			return "", errors.New("delegated question returned an invalid answer shape")
		}
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		DefaultClient: client, ImplementationIdentity: "native-waiting-delegate-test-build",
		ConfigurationIdentity: "native-waiting-delegate-test-config", DefaultMaxModelCalls: 4,
		BuildID: interactionTestBuildID,
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{question},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &nativeDelegateSessionStore{value: session.Session{
		ID: "session_1", Title: "waiting delegate", CWD: workspace,
		StartedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Revision: 1,
	}}
	projection := newNativeDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	coordinator := runs.NewCoordinator(runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: nativeDelegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(admission.Gate), Now: time.Now,
		NewRunID: func() string {
			id := runIDs[0]
			runIDs = runIDs[1:]
			return id
		},
		NewSegmentID: func() string {
			id := segmentIDs[0]
			segmentIDs = segmentIDs[1:]
			return id
		},
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_1",
		Capabilities: run.Capabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate waiting work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	initialEventsReady := make(chan []runs.Event, 1)
	go func() { initialEventsReady <- slices.Collect(started.Events) }()

	var barriers []runs.TreeBarrierCommit
	deadline := time.After(2 * time.Second)
	for len(barriers) == 0 {
		projection.mu.Lock()
		barriers = slices.Clone(projection.barriers)
		projection.mu.Unlock()
		if len(barriers) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("waiting Delegate did not publish a tree barrier")
		case <-time.After(time.Millisecond):
		}
	}
	projection.mu.Lock()
	reservationsBeforeRestore := len(projection.reservations)
	outcomesBeforeRestore := len(projection.outcomes)
	projection.mu.Unlock()
	if len(barriers) != 1 {
		t.Fatalf("waiting Delegate barriers = %d, want 1", len(barriers))
	}
	barrier := barriers[0]
	if len(barrier.Pending.Continuations) != 2 || len(barrier.Pending.Interrupts) != 1 ||
		len(barrier.Pending.Bindings) != 1 ||
		barrier.Pending.Interrupts[0].RunID != "run_child" {
		t.Fatalf("waiting Delegate boundary = %#v", barrier.Pending)
	}
	checkpointState, err := decodeInteractionCheckpointPayload(barrier.Checkpoint.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(checkpointState.tree.ProcessSnapshots()); got != 2 {
		t.Fatalf("checkpoint Process count = %d, want 2", got)
	}
	if err := executor.Release(t.Context(), runs.ExecutorRef{
		SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID,
	}); err != nil {
		t.Fatal(err)
	}

	continuation := waitingDelegateContinuation(barrier)
	ref, err := executor.StageContinuation(t.Context(), continuation)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := collectInteractionEvents(sequence)
	binding := barrier.Pending.Bindings[0]
	if err := executor.BeginContinuation(t.Context(), ref, []runs.InterruptAnswer{{
		InterruptItemID: binding.InterruptItemID, MemberID: binding.MemberID,
		RequestID:  binding.RequestID,
		Resolution: interrupt.Resolution{Answers: [][]string{{"restored value"}}},
	}}, barrier.Pending.Capabilities.InterruptKinds); err != nil {
		t.Fatal(err)
	}
	var observed []runs.ExecutorEvent
	select {
	case observed = <-eventsReady:
	case <-time.After(2 * time.Second):
		t.Fatal("restored waiting Delegate did not finish")
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	projection.mu.Lock()
	reservationsAfterRestore := len(projection.reservations)
	outcomesAfterRestore := len(projection.outcomes)
	projection.mu.Unlock()
	if reservationsBeforeRestore != 1 || outcomesBeforeRestore != 1 ||
		reservationsAfterRestore != reservationsBeforeRestore ||
		outcomesAfterRestore != outcomesBeforeRestore {
		t.Fatalf(
			"Delegate admission changed across restore: reservations %d→%d outcomes %d→%d",
			reservationsBeforeRestore, reservationsAfterRestore,
			outcomesBeforeRestore, outcomesAfterRestore,
		)
	}
	childEnd, parentToolEnd, rootEnd := -1, -1, -1
	for index, event := range observed {
		switch payload := event.Payload.(type) {
		case runs.SegmentEnded:
			if event.Member.MemberID == binding.MemberID {
				childEnd = index
			} else if !event.Member.Child() {
				rootEnd = index
			}
		case runs.ToolCallFinished:
			if !event.Member.Child() && payload.CallID != "" {
				parentToolEnd = index
			}
		}
	}
	if childEnd < 0 || parentToolEnd <= childEnd || rootEnd <= parentToolEnd {
		t.Fatalf(
			"restored Delegate order child-end=%d parent-tool=%d root-end=%d events=%#v",
			childEnd, parentToolEnd, rootEnd, observed,
		)
	}
	if model.Calls() != 4 {
		t.Fatalf("provider calls = %d, want 4 without replay", model.Calls())
	}
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-initialEventsReady:
	case <-time.After(time.Second):
		t.Fatal("initial waiting Delegate event stream did not close at shutdown")
	}
}
