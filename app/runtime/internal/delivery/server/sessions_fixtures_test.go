package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/models"
	planapp "github.com/Tangerg/lynx/app/runtime/internal/application/plans"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/core/chat"
)

var sessionFixtureSequence atomic.Uint64

func insertSessionFixture(
	ctx context.Context,
	store *sqlite.SessionStore,
	title, cwd string,
) (session.Session, error) {
	value, err := session.New(session.Draft{
		ID:    fmt.Sprintf("ses_fixture_%d", sessionFixtureSequence.Add(1)),
		Title: title, CWD: cwd, StartedAt: time.Now(),
	})
	if err != nil {
		return session.Session{}, err
	}
	if err := store.Insert(ctx, value); err != nil {
		return session.Session{}, err
	}
	return value, nil
}

func insertSessionSnapshot(
	ctx context.Context,
	store *sqlite.SessionStore,
	snapshot session.Snapshot,
) (session.Session, error) {
	value, err := session.Restore(snapshot)
	if err != nil {
		return session.Session{}, err
	}
	if err := store.Insert(ctx, value); err != nil {
		return session.Session{}, err
	}
	return value, nil
}

func forkSessionFixture(
	ctx context.Context,
	store *sqlite.SessionStore,
	parent session.Session,
) (session.Session, error) {
	child, err := parent.Fork(
		fmt.Sprintf("ses_fixture_%d", sessionFixtureSequence.Add(1)),
		"",
		time.Now(),
	)
	if err != nil {
		return session.Session{}, err
	}
	if err := store.Insert(ctx, child); err != nil {
		return session.Session{}, err
	}
	return child, nil
}

// testRuntime is the Delivery test seam newTestServer uses to build the Run
// coordinator: semantic execution plus a Run-segment effects factory. The test
// double stays on application-owned ports so handler fixtures do not depend on
// Agent adapter handles.
type testRuntime interface {
	runs.RootExecutionStarter
	runs.ExecutionObserver
	runs.ExecutionReleaser
	runs.WaitingExecutionContinuer
	runs.RunningExecutionSteerer
	runs.RunningSubtreeCanceler
	runs.WaitingSubtreeCancellationPreparer
	RunSegmentEffects() *runsegment.Effects
}

func serverPending(
	runID, sessionID, executorID, memberID string,
	open []transcript.Interrupt,
	createdAt time.Time,
) runs.Pending {
	if executorID == "" {
		executorID = "exec_" + runID
	}
	if memberID == "" {
		memberID = "member_" + runID
	}
	if createdAt.IsZero() {
		createdAt = time.Unix(1, 0).UTC()
	}
	if len(open) == 0 {
		open = []transcript.Interrupt{{
			ItemID: "interrupt_" + runID, ItemOccurredAt: createdAt,
			RunID:    runID,
			Kind:     interrupt.Question,
			Question: &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}},
		}}
	} else {
		open = append([]transcript.Interrupt(nil), open...)
		for index := range open {
			if open[index].RunID == "" {
				open[index].RunID = runID
			}
			if open[index].ItemOccurredAt.IsZero() {
				open[index].ItemOccurredAt = createdAt
			}
		}
	}
	bindings := make([]runs.InterruptBinding, len(open))
	capabilities := run.Capabilities{}
	for index, request := range open {
		capabilities.InterruptKinds = append(capabilities.InterruptKinds, request.Kind)
		bindings[index] = runs.InterruptBinding{
			InterruptItemID: request.ItemID,
			MemberID:        memberID,
			RequestID:       fmt.Sprintf("request_%s_%d", memberID, index),
		}
		if request.Kind == interrupt.Approval {
			bindings[index].ToolCallID = fmt.Sprintf("call_%s_%d", memberID, index)
		}
	}
	return runs.Pending{
		RootRunID:    runID,
		SessionID:    sessionID,
		ExecutorID:   executorID,
		Interrupts:   open,
		Bindings:     bindings,
		Capabilities: capabilities.Normalized(),
		Continuations: []runs.Continuation{{
			RunID:        runID,
			MemberID:     memberID,
			RunCreatedAt: createdAt,
		}},
		CreatedAt: createdAt,
	}
}

// executionRuntime is the combined application-owned execution surface this
// integration harness supplies. Production consumers still receive the two
// narrower interfaces independently.
type executionRuntime interface {
	runs.RootExecutionStarter
	runs.ExecutionObserver
	runs.ExecutionReleaser
	runs.WaitingExecutionContinuer
	runs.RunningExecutionSteerer
	runs.RunningSubtreeCanceler
	runs.WaitingSubtreeCancellationPreparer
}

// stubRuntime is the delivery session/lifecycle test double: it provides the run
// executor + effects (testRuntime) over its own in-memory + sqlite stores, and
// the coordinator-provider seams for Sessions, queries, and execution control.
type stubRuntime struct {
	sess        *sqlite.SessionStore
	model       string
	history     map[string][]chat.Message // per-session chat history (fork copies it)
	hist        *sqlite.TranscriptStore   // durable Item history
	runs        *sqlite.RunStore          // durable Run records (rollback/fork read runs)
	goals       *sqlite.GoalStore         // autonomous objective included in mounted material reads
	toolResults *sqlite.ToolResultStore
	plan        *sqlite.PlanStore                   // session-scoped state: exported, restored, dropped with the session
	interrupts  *persistence.InterruptStore         // open-interrupt registry (rollback clears dropped)
	muts        *persistence.WorkspaceMutationStore // §8.5 recoverable file-rollback log
	execution   executionRuntime
	admissions  *sessionadmission.Gate
}

// sessionsCoordinatorProvider is the optional test seam newTestServer uses to
// wire s.sessions: a fake that can build the real lifecycle coordinator over its
// own in-memory stores (stubRuntime). Fakes that never drive a lifecycle
// write-set may omit it, leaving s.sessions nil.
type sessionsCoordinatorProvider interface {
	sessionsCoordinator(admissions sessions.Admissions) *sessions.Coordinator
}

// queriesCoordinatorProvider is the parallel seam for the read coordinator: a
// fake that can build it over its own transcript and interrupt stores. Fakes
// that never drive a read (live-run tests) may omit it.
type queriesCoordinatorProvider interface {
	queriesCoordinator() *queries.Coordinator
}

// runProjectionProvider is the seam for the durable run read that addressing a
// live segment resolves through (subscribe / steer). Fakes with no run store may
// omit it; a handler that then addresses a segment fails loudly rather than
// guessing what the run is doing.
type runProjectionProvider interface {
	runProjection() runs.RunProjection
}

type conversationReaderProvider interface {
	conversationReader() runs.ConversationReader
}

type emptyConversationReader struct{}

func (emptyConversationReader) Read(context.Context, string) ([]chat.Message, error) {
	return nil, nil
}

type passthroughRunWorkingContext struct{}

func (passthroughRunWorkingContext) ComposeWorkingContext(
	_ context.Context,
	input runs.WorkingContextInput,
) ([]chat.Message, error) {
	return input.Seed, nil
}

type inertRunControl struct{}

func (inertRunControl) RequestRootCancellation(context.Context, runs.ExecutorRef, string) error {
	return nil
}

func (inertRunControl) RestoreWaitingExecution(
	context.Context,
	runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{}, errors.New("test runtime: waiting restoration is unavailable")
}

type inertRunProjection struct{}

func (inertRunProjection) Run(context.Context, string) (run.Run, bool, error) {
	return run.Run{}, false, nil
}

func (inertRunProjection) Tree(context.Context, string) ([]run.Run, error) {
	return nil, nil
}

type inertItemProjection struct{}

func (inertItemProjection) Item(context.Context, string) (transcript.Item, bool, error) {
	return transcript.Item{}, false, nil
}

func nonNilRunProjection(projection runs.RunProjection) runs.RunProjection {
	if projection == nil {
		return inertRunProjection{}
	}
	return projection
}

func itemProjectionFor(rt testRuntime) runs.ItemProjection {
	provider, ok := rt.(interface {
		Transcript() *sqlite.TranscriptStore
	})
	if !ok || provider.Transcript() == nil {
		return inertItemProjection{}
	}
	return provider.Transcript()
}

func (s stubRuntime) runProjection() runs.RunProjection {
	if s.runs == nil {
		return nil
	}
	return s.runs
}

func (s stubRuntime) conversationReader() runs.ConversationReader {
	return stubConversationReader{runtime: s}
}

type stubConversationReader struct{ runtime stubRuntime }

func (reader stubConversationReader) Read(ctx context.Context, id string) ([]chat.Message, error) {
	return reader.runtime.ReadHistory(ctx, id)
}

func (s stubRuntime) queriesCoordinator() *queries.Coordinator {
	return queries.New(queries.Dependencies{
		Transcript: s.hist,
		Interrupts: s.interrupts,
		Runs:       s.runs,
		Sessions:   s.sess,
		// The composition root passes the same store to both the write path and the
		// query one, because features.plan IS "the store exists" — so a harness that
		// wired only the writes could not reach plan.get at all.
		Plan: s.plan,
	})
}

func newTestServer(rt testRuntime) *Server {
	s := &Server{}
	admissions := &sessionadmission.Gate{}
	var sessionPorts runs.SessionPorts
	// Wire the session/run lifecycle coordinator over the fake's in-memory stores
	// when the fake provides one, mirroring the composition root.
	if p, ok := rt.(sessionsCoordinatorProvider); ok {
		sessionCoordinator := p.sessionsCoordinator(admissions)
		s.sessions = sessionCoordinator
		sessionPorts = runs.SessionPorts{
			Reader:       sessionCoordinator,
			Creator:      sessionCoordinator,
			ActiveRuns:   sessionCoordinator,
			Interrupts:   sessionCoordinator,
			Terminations: sessionCoordinator,
		}
	}
	var ids atomic.Uint64
	var runProjection runs.RunProjection
	if p, ok := rt.(runProjectionProvider); ok {
		runProjection = p.runProjection()
	}
	conversation := runs.ConversationReader(emptyConversationReader{})
	if p, ok := rt.(conversationReaderProvider); ok {
		conversation = p.conversationReader()
	}
	projectionWriter := rt.RunSegmentEffects()
	finalizer, err := runsegment.NewFinalizer(runsegment.FinalizerConfig{})
	if err != nil {
		panic(err)
	}
	workspaceNotifier := runsegment.NewWorkspaceNotifier(nil)
	runCoordinator, err := runs.NewCoordinator(runs.Dependencies{
		RootStarts:                         rt,
		Observations:                       rt,
		Releases:                           rt,
		RootCancellation:                   inertRunControl{},
		Conversation:                       conversation,
		WorkingContexts:                    passthroughRunWorkingContext{},
		Continuation:                       rt,
		WaitingRestorer:                    inertRunControl{},
		Steering:                           rt,
		RunningSubtreeCanceler:             rt,
		WaitingSubtreeCancellationPreparer: rt,
		Session:                            sessionPorts,
		Projection: runs.ProjectionPorts{
			Openings:                    projectionWriter,
			ChildStarts:                 projectionWriter,
			Checkpoints:                 projectionWriter,
			ResumeClaims:                projectionWriter,
			Events:                      projectionWriter,
			Barriers:                    projectionWriter,
			WaitingSubtreeCancellations: projectionWriter,
			Workspace:                   workspaceNotifier,
			Finalizer:                   finalizer,
		},
		Runs:       nonNilRunProjection(runProjection),
		Items:      itemProjectionFor(rt),
		Admissions: admissions,
		Now:        time.Now,
		NewRunID: func() string {
			return fmt.Sprintf("run_test_%d", ids.Add(1))
		},
		NewSegmentID: func() string {
			return fmt.Sprintf("seg_test_%d", ids.Add(1))
		},
	})
	if err != nil {
		panic(err)
	}
	s.runs = runCoordinator
	if p, ok := rt.(queriesCoordinatorProvider); ok {
		s.queries = p.queriesCoordinator()
	}
	// Capability handler tests replace this coordinator through serverWithModels;
	// session projection reads its complete model choice from the session use case.
	s.models = models.New(models.Config{})
	// Default to a disabled schedules coordinator (schedules.* report
	// capability_not_negotiated); schedule tests replace it with a fake registry.
	s.schedules = schedules.New(schedules.Dependencies{})
	return s
}

// serverWithModels builds a Server whose only wired coordinator is the models one
// — enough for the providers / models handler tests.
func serverWithModels(cfg models.Config) *Server {
	return &Server{models: models.New(cfg)}
}

// serverWithTools builds a Server whose only wired coordinator is the tools one —
// enough for the tools.* handler tests.
func serverWithTools(useCases toolUseCases) *Server {
	return &Server{tools: useCases}
}

func (s stubRuntime) Transcript() *sqlite.TranscriptStore     { return s.hist }
func (s stubRuntime) Interrupts() *persistence.InterruptStore { return s.interrupts }

// MessageCount and TruncateMessages operate on the in-memory history map,
// mirroring the engine's conversation-history store closely enough for
// rollback/fork tests.
func (s stubRuntime) MessageCount(_ context.Context, id string) (int, error) {
	return len(s.history[id]), nil
}

// RunInTx in the stub just runs fn; the in-memory stub has no real transaction,
// while production wires the sqlite-backed transactor.
func (s stubRuntime) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (s stubRuntime) TruncateMessages(_ context.Context, id string, keepN int) error {
	msgs := s.history[id]
	if keepN <= 0 {
		delete(s.history, id)
		return nil
	}
	if keepN < len(msgs) {
		s.history[id] = msgs[:keepN]
	}
	return nil
}

// executionStub supplies the default inert executor through application-owned
// values. Most Session tests never drive it, but complete methods keep an
// accidental call deterministic instead of dispatching through a nil embed.
type executionStub struct{}

func (executionStub) Observe(context.Context, runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	return func(func(runs.ExecutorEvent) bool) {}, nil
}
func (executionStub) ValidateRootStart(req runs.RootExecutionStart) error { return req.Validate() }
func (executionStub) StageRoot(_ context.Context, req runs.RootExecutionStart) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{SessionID: req.SessionID, ExecutorID: "exec_test"}, nil
}
func (executionStub) BeginRoot(context.Context, runs.ExecutorRef) error { return nil }
func (executionStub) StageContinuation(_ context.Context, continuation runs.WaitingContinuation) (runs.ExecutorRef, error) {
	return runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}, nil
}
func (executionStub) BeginContinuation(context.Context, runs.ExecutorRef, []runs.InterruptAnswer, []interrupt.Kind) error {
	return nil
}
func (executionStub) Release(context.Context, runs.ExecutorRef) error { return nil }
func (executionStub) CancelRunningSubtree(context.Context, runs.ExecutorRef, string, string) error {
	return nil
}
func (executionStub) PrepareWaitingSubtreeCancellation(
	context.Context,
	runs.WaitingSubtreeCancellationRequest,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	return runs.PreparedWaitingSubtreeCancellation{}, errors.New("test execution: waiting subtree cancellation is unavailable")
}
func (executionStub) SubmitSteer(context.Context, runs.ExecutorRef, []transcript.ContentBlock) error {
	return nil
}

type recordingExecutions struct {
	executionStub
	canceled []runs.ExecutorRef
}

func (r *recordingExecutions) Release(_ context.Context, ref runs.ExecutorRef) error {
	r.canceled = append(r.canceled, ref)
	return nil
}

func (s stubRuntime) executionController() executionRuntime {
	if s.execution != nil {
		return s.execution
	}
	return executionStub{}
}

func (s stubRuntime) Observe(ctx context.Context, ref runs.ExecutorRef) (iter.Seq[runs.ExecutorEvent], error) {
	return s.executionController().Observe(ctx, ref)
}

func (s stubRuntime) StageContinuation(ctx context.Context, continuation runs.WaitingContinuation) (runs.ExecutorRef, error) {
	return s.executionController().StageContinuation(ctx, continuation)
}

func (s stubRuntime) BeginContinuation(
	ctx context.Context,
	ref runs.ExecutorRef,
	answers []runs.InterruptAnswer,
	allowed []interrupt.Kind,
) error {
	return s.executionController().BeginContinuation(ctx, ref, answers, allowed)
}

func (s stubRuntime) ValidateRootStart(req runs.RootExecutionStart) error {
	return s.executionController().ValidateRootStart(req)
}

func (s stubRuntime) StageRoot(ctx context.Context, req runs.RootExecutionStart) (runs.ExecutorRef, error) {
	return s.executionController().StageRoot(ctx, req)
}

func (s stubRuntime) BeginRoot(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().BeginRoot(ctx, ref)
}

func (s stubRuntime) Cancel(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().Release(ctx, ref)
}

func (s stubRuntime) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	reason string,
) error {
	return s.executionController().CancelRunningSubtree(ctx, ref, memberID, reason)
}

func (s stubRuntime) PrepareWaitingSubtreeCancellation(
	ctx context.Context,
	request runs.WaitingSubtreeCancellationRequest,
) (runs.PreparedWaitingSubtreeCancellation, error) {
	return s.executionController().PrepareWaitingSubtreeCancellation(
		ctx,
		request,
	)
}

func (s stubRuntime) SubmitSteer(ctx context.Context, ref runs.ExecutorRef, input []transcript.ContentBlock) error {
	return s.executionController().SubmitSteer(ctx, ref, input)
}

func (s stubRuntime) Release(ctx context.Context, ref runs.ExecutorRef) error {
	return s.executionController().Release(ctx, ref)
}

type stubExecutionReleaser struct {
	rt *stubRuntime
}

func (t stubExecutionReleaser) Release(ctx context.Context, ref runs.ExecutorRef) error {
	return t.rt.Release(ctx, runs.ExecutorRef{SessionID: ref.SessionID, ExecutorID: ref.ExecutorID})
}

type stubLifecycleStores struct {
	rt *stubRuntime
}

func (s stubLifecycleStores) Session() sessions.Store { return s.rt.sess }

func (s stubLifecycleStores) Interrupts() sessions.InterruptStore { return s.rt.interrupts }

func (s stubLifecycleStores) Transcript() sessions.TranscriptStore { return s.rt.hist }

func (s stubLifecycleStores) Runs() sessions.RunStore { return s.rt.runs }

func (s stubLifecycleStores) ReadSnapshot(ctx context.Context, id string) (sessions.Snapshot, error) {
	ses, err := s.rt.sess.Get(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	items, err := s.rt.hist.List(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	runs, err := s.rt.runs.ListRuns(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	messages, err := s.rt.ReadHistory(ctx, id)
	if err != nil {
		return sessions.Snapshot{}, err
	}
	var toolResults []toolresult.Blob
	if s.rt.toolResults != nil {
		toolResults, err = s.rt.toolResults.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	var steps []plan.Step
	if s.rt.plan != nil {
		steps, err = s.rt.plan.List(ctx, id)
		if err != nil {
			return sessions.Snapshot{}, err
		}
	}
	return sessions.Snapshot{
		Session: ses, Messages: messages, Items: items, Runs: runs,
		ToolResults: toolResults, Plan: steps,
	}, nil
}

func (s stubLifecycleStores) ReadMaterialSnapshot(ctx context.Context, id string) (sessions.MaterialSnapshot, error) {
	ses, err := s.rt.sess.Get(ctx, id)
	if err != nil {
		return sessions.MaterialSnapshot{}, err
	}
	var items []transcript.Item
	if s.rt.hist != nil {
		items, err = s.rt.hist.List(ctx, id)
		if err != nil {
			return sessions.MaterialSnapshot{}, err
		}
	}
	var records []run.Run
	if s.rt.runs != nil {
		records, err = s.rt.runs.ListRuns(ctx, id)
		if err != nil {
			return sessions.MaterialSnapshot{}, err
		}
	}
	var pending []runs.Pending
	if s.rt.interrupts != nil {
		pending, err = s.rt.interrupts.List(ctx, id)
		if err != nil {
			return sessions.MaterialSnapshot{}, err
		}
	}
	var state plan.State
	if s.rt.plan != nil {
		state, err = s.rt.plan.State(ctx, id)
		if err != nil {
			return sessions.MaterialSnapshot{}, err
		}
	}
	var currentGoal *goal.Goal
	if s.rt.goals != nil {
		stored, found, err := s.rt.goals.Get(ctx, id)
		if err != nil {
			return sessions.MaterialSnapshot{}, err
		}
		if found {
			stored = stored.Clone()
			currentGoal = &stored
		}
	}
	return sessions.MaterialSnapshot{
		Session: ses, Items: items, Runs: records, Interrupts: pending, Plan: state,
		Goal: currentGoal,
	}, nil
}

func (s stubLifecycleStores) ApplyFork(ctx context.Context, plan sessions.ForkPlan) (session.Session, error) {
	child := plan.Child
	if err := child.Validate(); err != nil {
		return session.Session{}, err
	}
	if child.ParentID() != plan.ParentID || child.Revision() != 1 {
		return session.Session{}, errors.New("test persistence: invalid fork child")
	}
	if _, err := s.rt.sess.Get(ctx, plan.ParentID); err != nil {
		return session.Session{}, err
	}
	if err := s.rt.sess.Insert(ctx, child); err != nil {
		return session.Session{}, err
	}
	if err := s.rt.SeedHistory(ctx, child.ID(), plan.Messages); err != nil {
		return session.Session{}, err
	}
	for _, value := range plan.Runs {
		if err := s.rt.runs.Restore(ctx, value); err != nil {
			return session.Session{}, err
		}
	}
	for _, item := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, item); err != nil {
			return session.Session{}, err
		}
	}
	for _, blob := range plan.ToolResults {
		if s.rt.toolResults == nil {
			return session.Session{}, errors.New("test runtime: tool-result persistence is unavailable")
		}
		if err := s.rt.toolResults.Restore(ctx, blob); err != nil {
			return session.Session{}, err
		}
	}
	if s.rt.plan != nil && plan.PlanReplacement != nil {
		if err := s.rt.plan.Save(ctx, child.ID(), plan.PlanReplacement.ExpectedRevision(), plan.PlanReplacement.State()); err != nil {
			return session.Session{}, err
		}
	}
	return child, nil
}

// The atomic write-sets over the stub's in-memory chat log + real sqlite
// transcript/run/interrupt/session stores, mirroring the persistence adapter.
func (s stubLifecycleStores) ApplyRollback(ctx context.Context, plan sessions.RollbackPlan) error {
	if s.rt.plan != nil && plan.PlanReplacement != nil {
		if err := s.rt.plan.Save(ctx, plan.SessionID, plan.PlanReplacement.ExpectedRevision(), plan.PlanReplacement.State()); err != nil {
			return err
		}
	}
	if plan.KeepMessageMark >= 0 {
		if err := s.rt.TruncateMessages(ctx, plan.SessionID, plan.KeepMessageMark); err != nil {
			return err
		}
	}
	for _, runID := range plan.DropRunIDs {
		if err := s.rt.hist.DeleteRun(ctx, plan.SessionID, runID); err != nil {
			return err
		}
		if err := s.rt.runs.Delete(ctx, plan.SessionID, runID); err != nil {
			return err
		}
		if err := s.rt.interrupts.Delete(ctx, plan.SessionID, runID); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) ApplyRestore(ctx context.Context, plan sessions.RestorePlan) error {
	if err := plan.Session.Validate(); err != nil {
		return err
	}
	restored := plan.Session.State()
	id := restored.ID()
	if plan.Session.ExpectedRevision() == 0 {
		if err := s.rt.sess.Insert(ctx, restored); err != nil {
			return err
		}
	} else if err := s.rt.sess.Save(ctx, plan.Session.ExpectedRevision(), restored); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, id); err != nil {
		return err
	}
	if err := s.rt.hist.DeleteSession(ctx, id); err != nil {
		return err
	}
	if err := s.rt.runs.DeleteForSession(ctx, id); err != nil {
		return err
	}
	if s.rt.toolResults != nil {
		if err := s.rt.toolResults.DropSession(ctx, id); err != nil {
			return err
		}
	}
	if err := s.rt.TruncateMessages(ctx, id, 0); err != nil {
		return err
	}
	if err := s.rt.SeedHistory(ctx, id, plan.Messages); err != nil {
		return err
	}
	for _, r := range plan.Runs {
		if err := s.rt.runs.Restore(ctx, r); err != nil {
			return err
		}
	}
	for _, it := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, it); err != nil {
			return err
		}
	}
	for _, blob := range plan.ToolResults {
		if s.rt.toolResults == nil {
			return errors.New("test runtime: tool-result persistence is unavailable")
		}
		if err := s.rt.toolResults.Restore(ctx, blob); err != nil {
			return err
		}
	}
	// Replaced, never deleted-and-reinserted: the revision has to come out greater
	// than what this session already published.
	if s.rt.plan != nil && plan.PlanReplacement != nil {
		if err := s.rt.plan.Save(ctx, id, plan.PlanReplacement.ExpectedRevision(), plan.PlanReplacement.State()); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) ApplyDelete(ctx context.Context, plan sessions.DeletePlan) error {
	return s.deleteSession(ctx, plan.SessionID)
}

func (s stubLifecycleStores) deleteSession(ctx context.Context, sessionID string) error {
	if err := s.rt.hist.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rt.runs.DeleteForSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.rt.TruncateMessages(ctx, sessionID, 0); err != nil {
		return err
	}
	if err := s.deleteInterrupts(ctx, sessionID); err != nil {
		return err
	}
	if s.rt.toolResults != nil {
		if err := s.rt.toolResults.DropSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return s.rt.sess.Delete(ctx, sessionID)
}

func (s stubLifecycleStores) ApplyTerminal(ctx context.Context, plan sessions.TerminalPlan) error {
	root, ok := plan.RootRun()
	if !ok {
		return errors.New("terminal plan has no root Run")
	}
	for _, item := range plan.Items {
		if err := s.rt.hist.AppendItem(ctx, item); err != nil {
			return err
		}
	}
	if err := s.rt.interrupts.Delete(ctx, root.SessionID(), root.ID()); err != nil {
		return err
	}
	for _, record := range plan.Runs {
		if outcome, terminal := record.Outcome(); terminal && outcome == run.OutcomeLost {
			if err := s.rt.runs.RecoverLost(ctx, record); err != nil {
				return err
			}
			continue
		}
		if err := s.rt.runs.Terminalize(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (s stubLifecycleStores) deleteInterrupts(ctx context.Context, sessionID string) error {
	pending, err := s.rt.interrupts.List(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, p := range pending {
		if err := s.rt.interrupts.Delete(ctx, sessionID, p.RootRunID); err != nil {
			return err
		}
	}
	return nil
}

type stubMessageCounter struct{ rt stubRuntime }

func (s stubMessageCounter) Count(ctx context.Context, id string) (int, error) {
	return s.rt.MessageCount(ctx, id)
}
func (s stubMessageCounter) Write(ctx context.Context, id string, messages ...chat.Message) error {
	return s.rt.SeedHistory(ctx, id, messages)
}

// sessionsCoordinator builds the real lifecycle coordinator over the stub's
// in-memory stores and execution release, so newTestServer can wire s.sessions the way the
// composition root does — delivery drives every lifecycle write-set through it.
// File restore stays disabled (nil restorer); the checkpoint tests rebuild it
// with a real restorer via [stubRuntime.sessionsCoordinatorWithRestorer].
func (s *stubRuntime) sessionsCoordinator(admissions sessions.Admissions) *sessions.Coordinator {
	gate, ok := admissions.(*sessionadmission.Gate)
	if !ok {
		panic("test runtime requires sessionadmission.Gate")
	}
	s.admissions = gate
	return s.sessionsCoordinatorWithRestorer(nil, admissions)
}

func (s *stubRuntime) sessionsCoordinatorWithRestorer(checkpoints sessions.WorkspaceCheckpoints, shared ...sessions.Admissions) *sessions.Coordinator {
	admissions := sessions.Admissions(&sessionadmission.Gate{})
	if len(shared) > 0 && shared[0] != nil {
		admissions = shared[0]
	}
	stores := stubLifecycleStores{rt: s}
	var runStore sessions.RunStore = emptySessionRunStore{}
	if s.runs != nil {
		runStore = s.runs
	}
	deps := sessions.Dependencies{
		Sessions:          nonNilSessionStore(s.sess),
		Interrupts:        nonNilSessionInterrupts(s.interrupts),
		Transcript:        nonNilSessionTranscript(s.hist),
		Runs:              runStore,
		Snapshots:         stores,
		MaterialSnapshots: stores,
		Writes:            stores,
		Forgetter:         s,
		ExecutionReleaser: stubExecutionReleaser{rt: s},
		Paths:             workspacepath.Resolver{},
		Checkpoints:       checkpoints,
		Admissions:        admissions,
		Now:               time.Now,
		NewID: func() string {
			return fmt.Sprintf("ses_fixture_%d", sessionFixtureSequence.Add(1))
		},
		NewRunID: func() string {
			return runs.NewRunID(fmt.Sprintf("fixture_%d", sessionFixtureSequence.Add(1)))
		},
		NewItemID: func() string {
			return runs.NewItemID(fmt.Sprintf("fixture_%d", sessionFixtureSequence.Add(1)))
		},
		NewToolResultID: toolresult.NewID,
	}
	if s.plan != nil {
		deps.Plan = &sessions.PlanServices{
			Boundaries: s.plan,
			Replacements: planapp.New(planapp.Dependencies{
				Store: s.plan, Now: time.Now,
			}),
		}
	}
	if s.muts != nil {
		deps.Mutations = s.muts
	}
	coordinator, err := sessions.New(deps)
	if err != nil {
		panic(err)
	}
	return coordinator
}

// emptySessionRunStore is the explicit no-Run dependency for delivery fixtures
// that exercise Session CRUD without constructing SQLite Run persistence. A
// typed nil *sqlite.RunStore would satisfy the interface and panic on its first
// activity read, disguising a malformed fixture as an application failure.
type emptySessionRunStore struct{}

func (emptySessionRunStore) ListRuns(context.Context, string) ([]run.Run, error) {
	return nil, nil
}

func (emptySessionRunStore) ListNonTerminalRuns(context.Context) ([]run.Run, error) {
	return nil, nil
}

// inertRuntimeStores supplies explicit no-op collaborators to focused Delivery
// fixtures. Production constructors stay strict; a fixture that does not mount
// SQLite still names every capability instead of relying on a half-built object.
type inertRuntimeStores struct{}

func (inertRuntimeStores) List(context.Context) ([]session.Session, error) { return nil, nil }
func (inertRuntimeStores) ListPage(context.Context, bool, int64, string, int) ([]session.Session, error) {
	return nil, nil
}
func (inertRuntimeStores) Get(context.Context, string) (session.Session, error) {
	return session.Session{}, session.ErrNotFound
}
func (inertRuntimeStores) Insert(context.Context, session.Session) error { return nil }
func (inertRuntimeStores) Save(context.Context, uint64, session.Session) error {
	return nil
}
func (inertRuntimeStores) ListRuns(context.Context, string) ([]run.Run, error) { return nil, nil }
func (inertRuntimeStores) ListNonTerminalRuns(context.Context) ([]run.Run, error) {
	return nil, nil
}
func (inertRuntimeStores) Open(context.Context, runs.Pending) error { return nil }
func (inertRuntimeStores) Consume(context.Context, string, string) (runs.Pending, bool, error) {
	return runs.Pending{}, false, nil
}
func (inertRuntimeStores) Delete(context.Context, string, string) error { return nil }
func (inertRuntimeStores) GetInterrupt(context.Context, string) (runs.Pending, bool, error) {
	return runs.Pending{}, false, nil
}
func (inertRuntimeStores) ClaimResume(
	context.Context,
	string,
	string,
	[]runs.InterruptAnswer,
	time.Time,
) (runs.Pending, bool, error) {
	return runs.Pending{}, false, nil
}
func (inertRuntimeStores) RequireResumeClaim(context.Context, string, string) error { return nil }
func (inertRuntimeStores) AppendItem(context.Context, transcript.Item) error        { return nil }
func (inertRuntimeStores) Item(context.Context, string) (transcript.Item, bool, error) {
	return transcript.Item{}, false, nil
}
func (inertRuntimeStores) ReplaceItem(context.Context, transcript.Item, transcript.Item) error {
	return nil
}
func (inertRuntimeStores) StartModelInvocation(context.Context, string, string, string, string, time.Time) error {
	return nil
}
func (inertRuntimeStores) CompleteModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertRuntimeStores) FailModelInvocation(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertRuntimeStores) MarkModelInvocationUnknown(context.Context, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertRuntimeStores) StartToolInvocation(context.Context, string, string, string, string, string, time.Time) error {
	return nil
}
func (inertRuntimeStores) CompleteToolInvocation(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertRuntimeStores) MarkToolInvocationIncomplete(context.Context, string, string, string, string, string, time.Time, time.Time) error {
	return nil
}
func (inertRuntimeStores) SaveCheckpoint(context.Context, runs.ExecutorCheckpoint) error { return nil }
func (inertRuntimeStores) LoadCheckpoint(context.Context, string) (runs.ExecutorCheckpoint, error) {
	return runs.ExecutorCheckpoint{}, runs.ErrExecutorCheckpointNotFound
}
func (inertRuntimeStores) DeleteCheckpoints(context.Context, string, []string) error { return nil }
func (inertRuntimeStores) Reserve(context.Context, sqlite.ChildRunStartReservationRecord) error {
	return nil
}
func (inertRuntimeStores) Conclude(
	context.Context,
	sqlite.ChildRunStartReservationRecord,
	sqlite.ChildRunStartConclusion,
) (bool, error) {
	return true, nil
}
func (inertRuntimeStores) DeleteSession(context.Context, string) error { return nil }

type inertSessionInterrupts struct{ inertRuntimeStores }

func (inertSessionInterrupts) List(context.Context, string) ([]runs.Pending, error) {
	return nil, nil
}
func (inertSessionInterrupts) Get(context.Context, string) (runs.Pending, bool, error) {
	return runs.Pending{}, false, nil
}

type inertSessionTranscript struct{ inertRuntimeStores }

func (inertSessionTranscript) List(context.Context, string) ([]transcript.Item, error) {
	return nil, nil
}

func nonNilSessionStore(store *sqlite.SessionStore) sessions.Store {
	if store == nil {
		return inertRuntimeStores{}
	}
	return store
}

func nonNilSessionInterrupts(store *persistence.InterruptStore) sessions.InterruptStore {
	if store == nil {
		return inertSessionInterrupts{}
	}
	return store
}

func nonNilSessionTranscript(store *sqlite.TranscriptStore) sessions.TranscriptStore {
	if store == nil {
		return inertSessionTranscript{}
	}
	return store
}

func nonNilRunsegmentInterrupts(store *persistence.InterruptStore, fallback inertRuntimeStores) runsegment.InterruptStore {
	if store == nil {
		return fallback
	}
	return store
}

func nonNilRunsegmentResumeClaims(store *persistence.InterruptStore, fallback inertRuntimeStores) runsegment.ResumeClaimStore {
	if store == nil {
		return fallback
	}
	return store
}

func nonNilRunsegmentSessions(store *sqlite.SessionStore, fallback inertRuntimeStores) runsegment.SessionStore {
	if store == nil {
		return fallback
	}
	return store
}

func nonNilRunsegmentTranscript(store *sqlite.TranscriptStore, fallback inertRuntimeStores) runsegment.TranscriptStore {
	if store == nil {
		return fallback
	}
	return store
}

func nonNilRunsegmentItems(store *sqlite.TranscriptStore, fallback inertRuntimeStores) runsegment.ItemReplacer {
	if store == nil {
		return fallback
	}
	return store
}

func nonNilRunsegmentApprovals(store *sqlite.TranscriptStore, fallback inertRuntimeStores) runsegment.ToolApprovalStore {
	if store == nil {
		return fallback
	}
	return store
}

func runProgressFor(state runsegment.RunStore) runsegment.RunProgressWriter {
	progress, ok := state.(runsegment.RunProgressWriter)
	if !ok {
		panic("test Run store does not implement progress writes")
	}
	return progress
}

func (s stubRuntime) RunSegmentEffects() *runsegment.Effects {
	stores := inertRuntimeStores{}
	state := s.runStore()
	cfg := runsegment.Config{
		Interrupts:          nonNilRunsegmentInterrupts(s.interrupts, stores),
		ResumeClaims:        nonNilRunsegmentResumeClaims(s.interrupts, stores),
		Sessions:            nonNilRunsegmentSessions(s.sess, stores),
		Transcript:          nonNilRunsegmentTranscript(s.hist, stores),
		ItemReplacer:        nonNilRunsegmentItems(s.hist, stores),
		ToolApprovals:       nonNilRunsegmentApprovals(s.hist, stores),
		ModelInvocations:    stores,
		ToolInvocations:     stores,
		Conversation:        stubMessageCounter{rt: s},
		State:               state,
		RunProgress:         runProgressFor(state),
		ExecutorCheckpoints: stores,
		ChildRunStarts:      stores,
		Tx:                  s.RunInTx,
	}
	if s.toolResults != nil {
		cfg.ToolResults = s.toolResults
	}
	effects, err := runsegment.New(cfg)
	if err != nil {
		panic(err)
	}
	return effects
}

// runStore is the real Run table when the fixture has one, so a committed
// terminal actually lands where every Run read comes from. Fixtures that only
// exercise streaming keep the no-op.
func (s stubRuntime) runStore() runsegment.RunStore {
	if s.runs != nil {
		return s.runs
	}
	return stubRunState{}
}

type stubRunState struct{}

func (stubRunState) Run(context.Context, string) (run.Run, bool, error) {
	return run.Run{}, false, nil
}
func (stubRunState) Admit(context.Context, run.Draft) error { return nil }
func (stubRunState) Resume(
	context.Context,
	string,
	run.ResumeDraft,
	time.Time,
) error {
	return nil
}
func (stubRunState) RequireActiveSegment(context.Context, string, string, string) error {
	return nil
}
func (stubRunState) Suspend(context.Context, run.Run) error     { return nil }
func (stubRunState) Terminalize(context.Context, run.Run) error { return nil }
func (stubRunState) TerminalizeEvent(context.Context, run.Run, string, string) error {
	return nil
}
func (stubRunState) RecordRunCommit(context.Context, string, string, string, string) error {
	return nil
}
func (stubRunState) RecordWaitingRunCommit(context.Context, string, string, string) error {
	return nil
}
func (stubRunState) SuspendBarrier(context.Context, run.Run, string, string) error {
	return nil
}
func (stubRunState) RunCommitCommitted(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}
func (stubRunState) UpdateProgress(context.Context, string, string, string, run.Metrics, int64, time.Time) error {
	return nil
}

// ForgetSession is the no-op the session-delete / rollback / purge cascades call
// (via the lifecycle coordinator) to release a removed session's process-local
// gate; these tests have no live execution state to forget.
func (stubRuntime) ForgetSession(string) {}

func (s stubRuntime) ReadHistory(_ context.Context, id string) ([]chat.Message, error) {
	return s.history[id], nil
}
func (s stubRuntime) SeedHistory(_ context.Context, id string, msgs []chat.Message) error {
	if s.history != nil {
		s.history[id] = append(s.history[id], msgs...)
	}
	return nil
}

func newSessionServer(t *testing.T) (*Server, *sqlite.SessionStore, *stubRuntime) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := sqlite.NewSessionStore(db)
	// Interrupts is always wired in production (runtime composition root), and
	// the wire status now reads it (liveStatus), so give the stub a real store.
	runtime := &stubRuntime{sess: svc, model: "default-model", interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db))}
	return newTestServer(runtime), svc, runtime
}
