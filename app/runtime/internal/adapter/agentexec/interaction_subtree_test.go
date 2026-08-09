package agentexec

import (
	"context"
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

func TestInteractionExecutorAppliesColdWaitingDelegateCancellationWithoutDuplicateProjection(
	t *testing.T,
) {
	model := newWaitingDelegateModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask the user for the value required by delegated work.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		resolution, requireErr := interactioninput.Require(ctx, "delegate.required-value", runs.Interrupt{
			Kind: interrupt.Question,
			Question: &runs.QuestionPrompt{
				ToolName: "ask", Arguments: `{}`,
				Fields: []runs.QuestionFieldSpec{{Prompt: "Which value should the delegate use?"}},
			},
		})
		if requireErr != nil {
			return "", requireErr
		}
		return resolution.Answers[0][0], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		DefaultClient: client, ImplementationIdentity: "native-waiting-cancellation-test-build",
		ConfigurationIdentity: "native-waiting-cancellation-test-config", DefaultMaxModelCalls: 4,
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
		ID: "session_1", Title: "waiting cancellation", CWD: workspace,
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
		Capabilities: run.RunCapabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate waiting work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	initialEventsReady := make(chan []runs.Event, 1)
	go func() { initialEventsReady <- slices.Collect(started.Events) }()
	select {
	case events := <-initialEventsReady:
		if len(events) == 0 {
			t.Fatal("waiting Delegate produced no events")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiting Delegate did not park")
	}
	projection.mu.Lock()
	barriers := slices.Clone(projection.barriers)
	projection.mu.Unlock()
	if len(barriers) != 1 || len(barriers[0].Pending.Interrupts) != 1 ||
		len(barriers[0].Pending.Continuations) != 2 {
		t.Fatalf("waiting Delegate boundary = %#v", barriers)
	}
	barrier := barriers[0]
	ref := runs.ExecutorRef{SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	target := barrier.Pending.Interrupts[0]
	request := runs.WaitingSubtreeCancellationRequest{
		Continuation:   waitingDelegateContinuation(barrier),
		TargetMemberID: memberIDForRun(t, barrier.Pending, target.RunID),
		Reason:         "caller canceled the waiting delegate",
	}
	prepareCtx, cancelPrepare := context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err := executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if err := prepared.Validate(); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	cancelPrepare()
	if err := prepared.Change.Discard(); err != nil {
		t.Fatal(err)
	}
	liveSession, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	liveSession.mu.Lock()
	boundaryAfterDiscard := liveSession.boundary
	observerAfterDiscard := liveSession.observerWasAttached
	statusAfterDiscard := liveSession.process.Status()
	liveSession.mu.Unlock()
	if boundaryAfterDiscard != interactionBoundaryWaiting || observerAfterDiscard ||
		!isInteractionWaitingBoundary(statusAfterDiscard) {
		t.Fatalf(
			"discarded subtree boundary=%d observer=%t status=%s",
			boundaryAfterDiscard,
			observerAfterDiscard,
			statusAfterDiscard,
		)
	}

	prepareCtx, cancelPrepare = context.WithTimeout(t.Context(), 2*time.Second)
	prepared, err = executor.PrepareWaitingSubtreeCancellation(prepareCtx, request)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if len(prepared.CanceledMemberIDs) != 1 ||
		prepared.CanceledMemberIDs[0] != request.TargetMemberID ||
		len(prepared.PausedMemberIDs) != 1 ||
		prepared.PausedMemberIDs[0] != prepared.Checkpoint.RootMemberID ||
		len(prepared.PendingInterruptions) != 0 {
		cancelPrepare()
		t.Fatalf(
			"prepared waiting cancellation canceled=%v paused=%v interruptions=%d",
			prepared.CanceledMemberIDs,
			prepared.PausedMemberIDs,
			len(prepared.PendingInterruptions),
		)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	resumedEvents := collectInteractionEvents(sequence)
	if err := prepared.Change.Apply(runs.WaitingSubtreeResumesRunning); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	if calls := model.Calls(); calls != 2 {
		cancelPrepare()
		t.Fatalf("provider calls after state apply = %d, want 2 before continuation activation", calls)
	}
	if err := prepared.Change.Continue(t.Context()); err != nil {
		cancelPrepare()
		t.Fatal(err)
	}
	cancelPrepare()
	var observed []runs.ExecutorEvent
	select {
	case observed = <-resumedEvents:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not finish after waiting Delegate cancellation")
	}
	for _, event := range observed {
		if event.Member.MemberID == request.TargetMemberID {
			t.Fatalf("canceled Delegate leaked a duplicate executor projection: %#v", event)
		}
	}
	ended := payloadsOf[runs.SegmentEnded](observed)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal events = %#v, want one completed root", ended)
	}
	if model.Calls() != 3 {
		t.Fatalf("provider calls = %d, want 3 without canceled-child replay", model.Calls())
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func memberIDForRun(t *testing.T, pending runs.Pending, runID string) string {
	t.Helper()
	for _, continuation := range pending.Continuations {
		if continuation.RunID == runID {
			return continuation.MemberID
		}
	}
	t.Fatalf("Run %q has no waiting member", runID)
	return ""
}

func waitingDelegateContinuation(barrier runs.TreeBarrierCommit) runs.WaitingContinuation {
	members := make([]runs.WaitingMember, 0, len(barrier.Pending.Continuations))
	for _, member := range barrier.Pending.Continuations {
		members = append(members, runs.WaitingMember{
			RunID: member.RunID, MemberID: member.MemberID,
			ParentRunID: member.Lineage.ParentRunID, SpawnedByItemID: member.Lineage.SpawnedByItemID,
			ModelSelection: member.ModelSelection, Metrics: member.Metrics,
		})
	}
	return runs.WaitingContinuation{
		SessionID: barrier.Pending.SessionID, ExecutorID: barrier.Pending.ExecutorID,
		RootRunID: barrier.Pending.RootRunID, Members: members,
		Checkpoint: barrier.Checkpoint, Capabilities: barrier.Pending.Capabilities,
		ChildRunAdmissionEnabled: barrier.Pending.Capabilities.ChildRuns,
	}
}
