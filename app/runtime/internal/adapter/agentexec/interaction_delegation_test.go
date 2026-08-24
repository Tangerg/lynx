package agentexec

import (
	"context"
	"errors"
	"iter"
	"math"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func TestDelegateSubtreeBudgetReservesEveryRemainingProcessLevel(t *testing.T) {
	base := agent.Budget{Steps: 2, Effects: 3, Signals: 5}
	budget, err := delegateSubtreeBudget(base, 4)
	if err != nil {
		t.Fatal(err)
	}
	if budget != (agent.Budget{Steps: 8, Effects: 12, Signals: 20}) {
		t.Fatalf("scaled budget = %+v", budget)
	}
	if _, err := delegateSubtreeBudget(base, 0); err == nil {
		t.Fatal("zero process levels were accepted")
	}
	if _, err := delegateSubtreeBudget(
		agent.Budget{Steps: math.MaxUint64, Effects: 1, Signals: 1}, 2,
	); err == nil {
		t.Fatal("overflowing delegated subtree budget was accepted")
	}
}

func TestInteractionExecutorRunsDelegateAsProductChildRun(t *testing.T) {
	model := newDelegatingStubModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:      t.Context(),
		DefaultClient: client, DefaultSelection: testDefaultSelection(), ImplementationIdentity: "interaction-delegate-test-build",
		ConfigurationIdentity: "interaction-delegate-test-config", DefaultMaxModelCalls: 4,
		BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: sessionfixture.MustRestore(session.Snapshot{
		ID: "session_1", Title: "delegate", Workspace: sessionfixture.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(sessionadmission.Gate), Now: time.Now,
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
		SessionID:    "session_1",
		Capabilities: run.Capabilities{ChildRuns: true},
		Input:        []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "please delegate this work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := slices.Collect(started.Events)
	if len(events) == 0 {
		t.Fatal("Delegate produced no Run events")
	}
	childStarted := -1
	childFinished := -1
	parentToolFinished := -1
	rootFinished := -1
	childState := run.Running
	rootState := run.Running
	var childFailure, rootFailure *run.Failure
	for index, event := range events {
		switch payload := event.Payload.(type) {
		case runs.SegmentStarted:
			if event.RunID == "run_child" {
				childStarted = index
			}
		case runs.SegmentFinished:
			if event.RunID == "run_child" {
				childFinished = index
				childState = payload.Run.State()
				if failure, failed := payload.Run.Failure(); failed {
					childFailure = &failure
				}
			}
			if event.RunID == "run_root" {
				rootFinished = index
				rootState = payload.Run.State()
				if failure, failed := payload.Run.Failure(); failed {
					rootFailure = &failure
				}
			}
		case runs.ItemCompleted:
			invocation, present := payload.Item.ToolInvocation()
			if event.RunID == "run_root" && present && invocation.Name == "delegate_task" {
				parentToolFinished = index
			}
		}
	}
	if childStarted < 0 || childFinished <= childStarted || parentToolFinished <= childFinished ||
		rootFinished <= parentToolFinished {
		t.Fatalf(
			"Delegate order child-start=%d child-finish=%d parent-tool=%d root-finish=%d events=%#v",
			childStarted, childFinished, parentToolFinished, rootFinished, events,
		)
	}
	if childState != run.Completed || rootState != run.Completed {
		t.Fatalf(
			"Delegate terminal states child=%s error=%+v root=%s error=%+v",
			childState, childFailure, rootState, rootFailure,
		)
	}
	projection.mu.Lock()
	reservation := projection.reservations
	outcomes := projection.outcomes
	openings := slices.Clone(projection.openings)
	projection.mu.Unlock()
	if len(reservation) != 1 || len(outcomes) != 1 || len(openings) != 2 {
		t.Fatalf(
			"Delegate durability reservations=%d outcomes=%d openings=%d",
			len(reservation), len(outcomes), len(openings),
		)
	}
	if openings[1].Admit == nil || openings[1].Admit.RunID != "run_child" ||
		openings[1].Admit.ParentRunID != "run_root" ||
		openings[1].Admit.SpawnedByItemID == "" {
		t.Fatalf("managed child opening = %#v", openings[1])
	}
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorCancelsRunningDelegateAndKeepsRootRunning(t *testing.T) {
	model := newCancelableDelegateModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:      t.Context(),
		DefaultClient: client, DefaultSelection: testDefaultSelection(), ImplementationIdentity: "interaction-running-cancel-test-build",
		ConfigurationIdentity: "interaction-running-cancel-test-config", DefaultMaxModelCalls: 4,
		BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: sessionfixture.MustRestore(session.Snapshot{
		ID: "session_1", Title: "running cancellation", Workspace: sessionfixture.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	cancelAccepted := make(chan struct{})
	runningSubtreeCanceler := notifyingRunningSubtreeCanceler{
		inner: executor,
		accepted: func() {
			close(cancelAccepted)
		},
	}
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{}, RunningSubtreeCanceler: runningSubtreeCanceler,
		Session: runs.SessionPorts{
			Reader: sessions, Creator: sessions, ActiveRuns: sessions,
			Interrupts: sessions, Terminations: sessions,
		},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Runs: projection, Items: projection,
		Admissions: new(sessionadmission.Gate), Now: time.Now,
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
		SessionID: "session_1", Capabilities: run.Capabilities{ChildRuns: true},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate cancelable work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.Event, 1)
	go func() { eventsReady <- slices.Collect(started.Events) }()
	select {
	case <-model.childCallStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("delegated child did not enter its model call")
	}

	type cancellationResult struct {
		value runs.CancelResult
		err   error
	}
	canceled := make(chan cancellationResult, 1)
	go func() {
		value, cancelErr := coordinator.Cancel(context.Background(), runs.CancelCommand{
			RunID: "run_child", Reason: "caller canceled delegated work", AllowChildRun: true,
		})
		canceled <- cancellationResult{value: value, err: cancelErr}
	}()
	select {
	case <-cancelAccepted:
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation was not accepted")
	}
	select {
	case <-model.childCallReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation did not stop its in-flight model call")
	}

	var canceledResult runs.CancelResult
	select {
	case outcome := <-canceled:
		if outcome.err != nil {
			close(model.releaseRootContinuation)
			t.Fatalf("Cancel running Delegate: %v", outcome.err)
		}
		canceledResult = outcome.value
	case <-time.After(3 * time.Second):
		t.Fatal("running Delegate cancellation did not settle")
	}
	assertRunningDelegateCancellationResult(t, canceledResult)
	select {
	case <-model.rootContinuationStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not consume the canceled child result")
	}
	close(model.releaseRootContinuation)
	var events []runs.Event
	select {
	case events = <-eventsReady:
	case <-time.After(3 * time.Second):
		t.Fatal("root did not finish after child cancellation")
	}
	assertRunningDelegateCancellationEvents(t, events)
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertRunningDelegateCancellationResult(t *testing.T, result runs.CancelResult) {
	t.Helper()
	if result.Run.ID() != "run_child" || result.Run.State() != run.Canceled ||
		!runHasOutcome(result.Run, run.OutcomeCanceled) ||
		result.Run.Detail() != "caller canceled delegated work" {
		t.Fatalf("canceled child = %+v", result.Run)
	}
	if result.RootRun == nil || result.RootRun.ID() != "run_root" || result.RootRun.State() != run.Running {
		t.Fatalf("root after child cancellation = %+v, want running", result.RootRun)
	}
}

func runHasOutcome(record run.Run, expected run.Outcome) bool {
	outcome, terminal := record.Outcome()
	return terminal && outcome == expected
}

func assertRunningDelegateCancellationEvents(t *testing.T, events []runs.Event) {
	t.Helper()
	childFinished, rootFinished := false, false
	for _, event := range events {
		finished, ok := event.Payload.(runs.SegmentFinished)
		if !ok {
			continue
		}
		if event.RunID == "run_child" {
			childFinished = finished.Run.State() == run.Canceled &&
				finished.Run.Detail() == "caller canceled delegated work"
		}
		if event.RunID == "run_root" {
			rootFinished = finished.Run.State() == run.Completed
		}
	}
	if !childFinished || !rootFinished {
		t.Fatalf("terminal projection child=%t root=%t events=%#v", childFinished, rootFinished, events)
	}
}

func TestInteractionExecutorProjectsConcurrentDelegateSiblingsExactlyOnce(t *testing.T) {
	result := runDelegateTree(t, newOrderedSiblingDelegateModel(), "run siblings", 3)
	rootID := result.rootRunID(t)
	directChildren := 0
	for _, opening := range result.openings {
		if opening.Admit == nil || opening.Admit.RunID == rootID {
			continue
		}
		if opening.Admit.ParentRunID != rootID || opening.Admit.RootRunID != rootID {
			t.Fatalf("sibling opening has invalid lineage: %+v", opening.Admit)
		}
		directChildren++
	}
	if directChildren != 2 {
		t.Fatalf("direct child openings = %d, want 2", directChildren)
	}
	result.assertAllRunsCompleted(t)
}

type orderedSiblingDelegateModel struct {
	defaults  *chat.Options
	bReturned chan struct{}
	bOnce     sync.Once
}

func newOrderedSiblingDelegateModel() *orderedSiblingDelegateModel {
	return &orderedSiblingDelegateModel{
		defaults:  &chat.Options{Model: "stub-ordered-sibling-delegate"},
		bReturned: make(chan struct{}),
	}
}

func (model *orderedSiblingDelegateModel) DefaultOptions() chat.Options { return *model.defaults }

func (model *orderedSiblingDelegateModel) Call(
	ctx context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	switch {
	case hasToolMessage(request.Messages):
		return interactionUsageTextResponse("root: siblings done", 2, 1), nil
	case userMessagesContain(request.Messages, "sibling A"):
		select {
		case <-model.bReturned:
			return interactionUsageTextResponse("child: sibling A", 2, 1), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case userMessagesContain(request.Messages, "sibling B"):
		model.bOnce.Do(func() { close(model.bReturned) })
		return interactionUsageTextResponse("child: sibling B", 2, 1), nil
	case userMessagesContain(request.Messages, "run siblings"):
		return interactionToolBatchResponse([]chat.ToolCall{
			{ID: "delegate_a", Name: "delegate_task", Arguments: `{"summary":"sibling A","instructions":"sibling A"}`},
			{ID: "delegate_b", Name: "delegate_task", Arguments: `{"summary":"sibling B","instructions":"sibling B"}`},
		}, 2, 1), nil
	default:
		return nil, errors.New("unexpected ordered sibling Delegate model context")
	}
}

func (model *orderedSiblingDelegateModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	response, err := model.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func TestInteractionExecutorProjectsNestedDelegateLineageExactlyOnce(t *testing.T) {
	result := runDelegateTree(t, newNestedDelegatingStub(), "nested root", 3)
	rootID := result.rootRunID(t)
	children := make(map[string]string)
	for _, opening := range result.openings {
		if opening.Admit == nil || opening.Admit.RunID == rootID {
			continue
		}
		children[opening.Admit.RunID] = opening.Admit.ParentRunID
		if opening.Admit.RootRunID != rootID {
			t.Fatalf("nested opening root = %q, want %q", opening.Admit.RootRunID, rootID)
		}
	}
	if len(children) != 2 {
		t.Fatalf("nested child openings = %v, want child and grandchild", children)
	}
	var childID string
	for runID, parentID := range children {
		if parentID == rootID {
			childID = runID
		}
	}
	if childID == "" {
		t.Fatalf("nested lineage has no direct child of %q: %v", rootID, children)
	}
	grandchildren := 0
	for _, parentID := range children {
		if parentID == childID {
			grandchildren++
		}
	}
	if grandchildren != 1 {
		t.Fatalf("nested lineage = %v, want one grandchild of %q", children, childID)
	}
	result.assertAllRunsCompleted(t)
}

type delegateTreeResult struct {
	events   []runs.Event
	openings []runs.OpeningCommit
}

func runDelegateTree(
	t *testing.T,
	model chat.Model,
	input string,
	wantProcesses int,
) delegateTreeResult {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:      t.Context(),
		DefaultClient: client, DefaultSelection: testDefaultSelection(), ImplementationIdentity: "interaction-delegate-tree-test-build",
		ConfigurationIdentity: "interaction-delegate-tree-test-config", DefaultMaxModelCalls: 6,
		MaxConcurrentToolCalls: 4, BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: sessionfixture.MustRestore(session.Snapshot{
		ID: "session_tree", Title: "delegate tree", Workspace: sessionfixture.MustWorkspace(workspace),
	})}
	projection := newDelegateProjection()
	var identityMu sync.Mutex
	runSequence, segmentSequence := 0, 0
	coordinator := mustNewRunCoordinator(t, runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(sessionadmission.Gate), Now: time.Now,
		NewRunID: func() string {
			identityMu.Lock()
			defer identityMu.Unlock()
			runSequence++
			return "run_tree_" + strconv.Itoa(runSequence)
		},
		NewSegmentID: func() string {
			identityMu.Lock()
			defer identityMu.Unlock()
			segmentSequence++
			return "segment_tree_" + strconv.Itoa(segmentSequence)
		},
	})
	started, err := coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_tree", Capabilities: run.Capabilities{ChildRuns: true},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: input}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := slices.Collect(started.Events)
	coordinator.BeginShutdown()
	if err := coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	projection.mu.Lock()
	openings := slices.Clone(projection.openings)
	projection.mu.Unlock()
	if len(openings) != wantProcesses {
		t.Fatalf("delegate tree openings = %d, want %d", len(openings), wantProcesses)
	}
	return delegateTreeResult{events: events, openings: openings}
}

func (result delegateTreeResult) rootRunID(t *testing.T) string {
	t.Helper()
	for _, opening := range result.openings {
		if opening.Admit != nil && opening.Admit.ParentRunID == "" {
			return opening.Admit.RunID
		}
	}
	t.Fatal("delegate tree has no root opening")
	return ""
}

func (result delegateTreeResult) assertAllRunsCompleted(t *testing.T) {
	t.Helper()
	completed := make(map[string]int, len(result.openings))
	for _, event := range result.events {
		finished, ok := event.Payload.(runs.SegmentFinished)
		if !ok {
			continue
		}
		if finished.Run.State() != run.Completed {
			t.Fatalf(
				"Run %q finished as %s outcome=%v detail=%q error=%+v",
				finished.Run.ID(), finished.Run.State(), finished.Run.Snapshot().Outcome, finished.Run.Detail(), finished.Run.Snapshot().Failure,
			)
		}
		completed[finished.Run.ID()]++
	}
	if len(completed) != len(result.openings) {
		t.Fatalf("completed Runs = %v, openings = %d", completed, len(result.openings))
	}
	for runID, count := range completed {
		if count != 1 {
			t.Fatalf("Run %q terminal projections = %d, want 1", runID, count)
		}
	}
}
