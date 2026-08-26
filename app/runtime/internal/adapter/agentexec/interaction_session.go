package agentexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type interactionSession struct {
	ref        runs.ExecutorRef
	scope      runs.ExecutionScope
	deployment agent.Deployment
	input      agent.Input
	engine     *agent.Engine

	lifetime            interactionLifetime
	state               interactionState
	childProjection     interactionChildProjection
	accounting          interactionAccounting
	unknownPollInterval time.Duration
	statePollInterval   time.Duration
	mcpToolAutoApproved func(server, tool string) bool
	maintenance         RunMaintenance
	lifecycleHooks      InteractionLifecycleHooks
	buildID             string
	start               runs.RootExecutionStart
	toolOutcomes        interactionToolOutcomes
	committedReplies    interactionCommittedReplies
	segmentClock        interactionSegmentClock
}

// interactionState owns the one lock domain whose facts must move atomically:
// the live Process, its observation/waiting boundary, exact pending steers,
// Delegate topology, and cancellation plane. Accounting, repetition detection,
// committed replies, and Segment timing have independent invariants and do not
// belong under this lock.
type interactionState struct {
	mu                        sync.Mutex
	pendingSteers             map[agent.SignalID]pendingInteractionSteer
	process                   *agent.Process
	admittedProcessID         agent.ProcessID
	observerWasAttached       bool
	begun                     bool
	finished                  bool
	boundary                  interactionBoundary
	waitingCheckpoint         runs.ExecutorCheckpoint
	subtreeChange             *interactionWaitingSubtreeChange
	subtreePrepared           chan struct{}
	unknownReported           bool
	deployments               *interactionDeploymentSet
	delegateCalls             map[delegateCallIdentity]*managedDelegateCall
	delegateChildren          map[agent.ProcessID]*managedDelegateCall
	activeDispatches          map[string]activeInteractionDispatch
	canceledSubtreeRoots      map[agent.ProcessID]struct{}
	rootCancellationRequested bool
}

type activeInteractionDispatch struct {
	processID agent.ProcessID
	cancel    context.CancelCauseFunc
}

type pendingInteractionSteer struct {
	content []transcript.ContentBlock
}

type interactionBoundary uint8

const (
	interactionBoundaryInactive interactionBoundary = iota
	interactionBoundaryWaiting
	interactionBoundaryContinuationStaged
	interactionBoundarySubtreePreparing
	interactionBoundarySubtreePrepared
	interactionBoundarySubtreeApplying
	interactionBoundarySubtreeApplied
	interactionBoundarySubtreeRecovery
)

func newInteractionSession(
	lifetime context.Context,
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
	config InteractionExecutorConfig,
) *interactionSession {
	provider := start.ModelSelection.Provider()
	if provider == "" {
		provider = config.Provider
	}
	return &interactionSession{
		ref: ref, scope: rootExecutionScope(start), lifetime: newInteractionLifetime(lifetime),
		state: interactionState{
			pendingSteers:        make(map[agent.SignalID]pendingInteractionSteer),
			delegateCalls:        make(map[delegateCallIdentity]*managedDelegateCall),
			delegateChildren:     make(map[agent.ProcessID]*managedDelegateCall),
			activeDispatches:     make(map[string]activeInteractionDispatch),
			canceledSubtreeRoots: make(map[agent.ProcessID]struct{}),
		},
		committedReplies: newInteractionCommittedReplies(),
		accounting: newInteractionAccounting(
			provider,
			start.ModelSelection.Model(),
			config.Pricing,
		),
		unknownPollInterval: config.UnknownEffectPollInterval,
		statePollInterval:   config.StatePollInterval,
		mcpToolAutoApproved: config.MCPToolAutoApproved,
		maintenance:         config.Maintenance,
		lifecycleHooks:      config.LifecycleHooks,
		buildID:             config.BuildID, start: start,
	}
}

var errInteractionRunCanceled = errors.New("agentexec: Interaction Run cancellation requested")

func interactionDispatchKey(request agent.EffectRequest) string {
	return request.ProcessID().String() + "\x00" + request.ID().String()
}

// beginDispatch binds one Agent-owned Effect attempt to the product Run's
// explicit cancellation plane. Agent Framework deliberately lets an in-flight
// Effect settle before applying a cancellation intent; this adapter-owned
// context gives cooperative model and Tool implementations a chance to produce
// that settlement promptly without changing Framework lifecycle semantics.
func (session *interactionSession) beginDispatch(
	ctx context.Context,
	request agent.EffectRequest,
) (context.Context, func()) {
	bound, cancel := context.WithCancelCause(ctx)
	stopLifetimeBinding := context.AfterFunc(session.lifetime.context, func() {
		cancel(context.Cause(session.lifetime.context))
	})
	key := interactionDispatchKey(request)
	session.state.mu.Lock()
	if session.state.rootCancellationRequested || session.inCanceledSubtreeLocked(request.ProcessID()) {
		cancel(errInteractionRunCanceled)
	} else {
		session.state.activeDispatches[key] = activeInteractionDispatch{
			processID: request.ProcessID(),
			cancel:    cancel,
		}
	}
	session.state.mu.Unlock()
	return bound, func() {
		session.state.mu.Lock()
		delete(session.state.activeDispatches, key)
		session.state.mu.Unlock()
		stopLifetimeBinding()
		cancel(nil)
	}
}

func (session *interactionSession) cancelAllDispatches() {
	session.state.mu.Lock()
	session.state.rootCancellationRequested = true
	cancels := make([]context.CancelCauseFunc, 0, len(session.state.activeDispatches))
	for _, dispatch := range session.state.activeDispatches {
		cancels = append(cancels, dispatch.cancel)
	}
	session.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (session *interactionSession) cancelSubtreeDispatches(rootID agent.ProcessID) {
	session.state.mu.Lock()
	session.state.canceledSubtreeRoots[rootID] = struct{}{}
	cancels := make([]context.CancelCauseFunc, 0, len(session.state.activeDispatches))
	for _, dispatch := range session.state.activeDispatches {
		if session.inSubtreeLocked(dispatch.processID, rootID) {
			cancels = append(cancels, dispatch.cancel)
		}
	}
	session.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (session *interactionSession) inCanceledSubtreeLocked(processID agent.ProcessID) bool {
	for rootID := range session.state.canceledSubtreeRoots {
		if session.inSubtreeLocked(processID, rootID) {
			return true
		}
	}
	return false
}

func (session *interactionSession) inSubtreeLocked(
	processID agent.ProcessID,
	rootID agent.ProcessID,
) bool {
	for range len(session.state.delegateChildren) + 1 {
		if processID == rootID {
			return true
		}
		managed := session.state.delegateChildren[processID]
		if managed == nil {
			return false
		}
		processID = managed.identity.parentID
	}
	return false
}

func interactionSegmentDuration(
	processStartedAt time.Time,
	segmentStartedAt time.Time,
	finishedAt time.Time,
) time.Duration {
	startedAt := processStartedAt
	if segmentStartedAt.After(startedAt) {
		startedAt = segmentStartedAt
	}
	return max(finishedAt.Sub(startedAt), 0)
}

func (state *interactionState) attachObserver() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.observerWasAttached || state.finished {
		return false
	}
	if state.begun && state.boundary != interactionBoundaryContinuationStaged &&
		state.boundary != interactionBoundarySubtreePrepared {
		return false
	}
	state.observerWasAttached = true
	return true
}

func (state *interactionState) detachObserver() {
	state.mu.Lock()
	state.observerWasAttached = false
	state.mu.Unlock()
}

func (state *interactionState) observerAttached() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.observerWasAttached
}

func (state *interactionState) begin() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.begun || state.finished {
		return false
	}
	state.begun = true
	return true
}

func (state *interactionState) setProcess(process *agent.Process) {
	state.mu.Lock()
	state.process = process
	state.mu.Unlock()
}

func (state *interactionState) processHandle() *agent.Process {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.process
}

func (session *interactionSession) submitSteer(
	ctx context.Context,
	message corechat.Message,
	content []transcript.ContentBlock,
) error {
	if ctx == nil {
		return errors.New("agentexec: steer context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	process := session.state.processHandle()
	if process == nil {
		return runs.ErrExecutorNotLive
	}
	signalID, err := agent.ParseSignalID("steer:" + uuid.NewString())
	if err != nil {
		return fmt.Errorf("agentexec: construct Interaction steer identity: %w", err)
	}
	signal, err := interaction.NewSteerSignal(signalID, message)
	if err != nil {
		return fmt.Errorf("agentexec: construct Interaction steer Signal: %w", err)
	}
	session.state.mu.Lock()
	session.state.pendingSteers[signalID] = pendingInteractionSteer{
		content: transcript.CloneContent(content),
	}
	session.state.mu.Unlock()
	accepted, deliverErr := process.DeliverSignal(
		runExecutionContext(ctx, session.scope, session.start), signal,
	)
	if deliverErr != nil {
		// A context error only reports that the caller stopped waiting. Engine may
		// already have accepted the command, so retain its exact product mapping
		// until ModelInvocation attributes it or the session is released.
		if !errors.Is(deliverErr, context.Canceled) &&
			!errors.Is(deliverErr, context.DeadlineExceeded) {
			session.removePendingSteer(signalID)
		}
		return fmt.Errorf("agentexec: deliver Interaction steer Signal: %w", deliverErr)
	}
	if !accepted {
		session.removePendingSteer(signalID)
		return errors.New("agentexec: Interaction steer Signal was not accepted")
	}
	return nil
}

func (session *interactionSession) removePendingSteer(signalID agent.SignalID) {
	session.state.mu.Lock()
	delete(session.state.pendingSteers, signalID)
	session.state.mu.Unlock()
}

func (session *interactionSession) commitAppliedSteers(
	ctx context.Context,
	member runs.ExecutorMember,
	signalIDs []agent.SignalID,
) error {
	if len(signalIDs) == 0 {
		return nil
	}
	session.state.mu.Lock()
	messages := make([][]transcript.ContentBlock, len(signalIDs))
	seen := make(map[agent.SignalID]struct{}, len(signalIDs))
	for index, signalID := range signalIDs {
		if _, duplicate := seen[signalID]; duplicate {
			session.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution repeats steer Signal %s", signalID)
		}
		seen[signalID] = struct{}{}
		pending, found := session.state.pendingSteers[signalID]
		if !found {
			session.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution names unknown steer Signal %s", signalID)
		}
		messages[index] = transcript.CloneContent(pending.content)
	}
	session.state.mu.Unlock()
	if err := session.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
		return fmt.Errorf("agentexec: commit applied Interaction steers: %w", err)
	}
	session.state.mu.Lock()
	for _, signalID := range signalIDs {
		delete(session.state.pendingSteers, signalID)
	}
	session.state.mu.Unlock()
	return nil
}

func (session *interactionSession) startWorkers() {
	session.lifetime.workers.Add(1)
	go func() {
		defer session.lifetime.workers.Done()
		session.await()
	}()
	session.lifetime.reconcilers.Add(2)
	go func() {
		defer session.lifetime.reconcilers.Done()
		session.reconcileUnknownEffects()
	}()
	go func() {
		defer session.lifetime.reconcilers.Done()
		session.reconcileExecutionState()
	}()
}

func (session *interactionSession) failStart() {
	session.finish()
}

func (session *interactionSession) stopReconciliation() {
	session.lifetime.stop()
	session.lifetime.reconcilers.Wait()
}

func (session *interactionSession) finish() {
	session.lifetime.finishOnce.Do(func() {
		session.state.mu.Lock()
		session.state.finished = true
		session.state.mu.Unlock()
		session.stopReconciliation()
		close(session.lifetime.events)
		close(session.lifetime.done)
	})
}

func (session *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		return
	}
	response := parsed.Response()
	if response.Output == nil || response.Output.Message == nil {
		return
	}
	for _, part := range response.Output.Message.Parts {
		var payload runs.ExecutionFact
		switch part.Kind {
		case corechat.PartText:
			payload = runs.MessageDelta{Text: part.Text}
		case corechat.PartReasoning:
			payload = runs.ReasoningDelta{Text: part.Text}
		default:
			continue
		}
		member, found := session.executorMemberByProcessID(delta.ProcessID())
		if found && session.lifetime.offer(runs.ExecutorEvent{Member: member, Payload: payload}) {
			continue
		}
		trace.SpanFromContext(ctx).AddEvent(
			"agentexec.delta.dropped",
			trace.WithAttributes(attribute.String("process.id", delta.ProcessID().String())),
		)
	}
}

func (session *interactionSession) flushDeltas(ctx context.Context) error {
	if session.engine == nil {
		return errors.New("agentexec: Interaction engine is unavailable")
	}
	if err := session.engine.FlushDeltas(ctx); err != nil {
		return fmt.Errorf("agentexec: flush model deltas: %w", err)
	}
	return nil
}

func (session *interactionSession) observeFrameworkEvent(_ context.Context, event agent.Event) {
	if event.Relation().RootID() != session.processRootID() {
		return
	}
	session.lifetime.wakeState()
}

func (session *interactionSession) processRootID() agent.ProcessID {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.process == nil {
		return agent.ProcessID{}
	}
	return session.state.process.Relation().RootID()
}

func (session *interactionSession) commitFact(
	ctx context.Context,
	member runs.ExecutorMember,
	fact runs.ExecutionFact,
) error {
	ctx, cancel := session.lifetime.bind(ctx)
	defer cancel()
	commit, receipt, err := runs.NewExecutionFactCommit(fact)
	if err != nil {
		return err
	}
	event := runs.ExecutorEvent{Member: member, Payload: commit}
	if err := session.lifetime.sendAuthoritative(ctx, event); err != nil {
		return err
	}
	return receipt.Await(ctx)
}

func (session *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(session.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.lifetime.unknownWake:
		case <-ticker.C:
		case <-session.lifetime.context.Done():
			return
		}
		if session.reportUnknownEffects() {
			return
		}
	}
}

func (session *interactionSession) reconcileExecutionState() {
	ticker := time.NewTicker(session.statePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.lifetime.stateWake:
		case <-ticker.C:
		case <-session.lifetime.context.Done():
			return
		}
		ctx, cancel := context.WithTimeout(session.lifetime.context, authoritativeProjectionTimeout)
		progressed, err := session.reconcileCompletedDelegateChildren(ctx)
		cancel()
		if err != nil {
			session.publishProjectionFailure(err)
			return
		}
		if progressed {
			continue
		}
		if session.publishWaitingBoundary() {
			continue
		}
	}
}

func (session *interactionSession) publishWaitingBoundary() bool {
	session.state.mu.Lock()
	process := session.state.process
	if process == nil || session.state.finished || session.state.boundary != interactionBoundaryInactive {
		session.state.mu.Unlock()
		return false
	}
	session.state.mu.Unlock()
	if process.Status() != agent.StatusWaiting {
		return false
	}
	ctx, cancel := context.WithTimeout(session.lifetime.context, authoritativeProjectionTimeout)
	defer cancel()
	snapshot, interruptions, found, err := session.captureHumanInputBarrier(ctx)
	if err != nil {
		session.publishProjectionFailure(err)
		return false
	}
	if !found {
		return false
	}
	checkpoint, err := session.executorCheckpoint(snapshot)
	if err != nil {
		session.publishProjectionFailure(err)
		return false
	}
	session.state.mu.Lock()
	if session.state.finished || session.state.boundary != interactionBoundaryInactive ||
		session.state.process != process || process.Status() != agent.StatusWaiting {
		session.state.mu.Unlock()
		return false
	}
	session.state.boundary = interactionBoundaryWaiting
	session.state.waitingCheckpoint = checkpoint.Clone()
	session.state.mu.Unlock()
	published := session.lifetime.send(runs.ExecutorEvent{
		Member: session.executorMember(process.Relation()),
		Payload: runs.TreeInterrupted{
			Checkpoint: checkpoint, Interruptions: interruptions,
		},
	})
	if published && session.lifecycleHooks != nil {
		session.lifecycleHooks.NotifyWaiting(
			session.lifetime.context, session.start.SessionID, session.start.CWD,
		)
	}
	return published
}

func (session *interactionSession) stageContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.finished || session.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if session.state.boundary != interactionBoundaryWaiting || session.state.observerWasAttached ||
		!isInteractionWaitingBoundary(session.state.process.Status()) {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(session.state.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	session.state.boundary = interactionBoundaryContinuationStaged
	return nil
}

func (session *interactionSession) beginContinuation(allowedInterrupts []interrupt.Kind) error {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.finished || session.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if session.state.boundary != interactionBoundaryContinuationStaged || !session.state.observerWasAttached ||
		!isInteractionWaitingBoundary(session.state.process.Status()) {
		return errors.New("agentexec: Interaction continuation was not staged and observed")
	}
	if !slices.Equal(session.start.InterruptKinds, allowedInterrupts) {
		return errors.New("agentexec: continuation capabilities differ from the staged Interaction")
	}
	return nil
}

func isInteractionWaitingBoundary(status agent.Status) bool {
	return status == agent.StatusWaiting || status == agent.StatusPaused
}

func (session *interactionSession) continuationAccepted() {
	session.state.mu.Lock()
	session.state.boundary = interactionBoundaryInactive
	session.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
	session.state.mu.Unlock()
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.Scope == right.Scope && left.ModelSelection == right.ModelSelection &&
		left.Limits == right.Limits && slices.Equal(left.Usage.Models, right.Usage.Models) &&
		bytes.Equal(left.Payload, right.Payload)
}

func (session *interactionSession) reportUnknownEffects() bool {
	ctx, cancel := context.WithTimeout(session.lifetime.context, authoritativeProjectionTimeout)
	defer cancel()
	ids, err := session.unknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = id.String()
	}
	slices.Sort(values)
	values = slices.Compact(values)
	session.state.mu.Lock()
	if session.state.unknownReported {
		session.state.mu.Unlock()
		return true
	}
	session.state.unknownReported = true
	member := runs.ExecutorMember{MemberID: session.state.process.Relation().ProcessID().String()}
	session.state.mu.Unlock()
	return session.lifetime.send(runs.ExecutorEvent{
		Member: member, Payload: runs.UnknownEffectsDetected{IDs: values},
	})
}

func (session *interactionSession) await() {
	joinCtx := context.WithoutCancel(session.lifetime.context)
	result, err := session.state.process.Await(joinCtx)
	if err == nil {
		projectionCtx, cancel := context.WithTimeout(joinCtx, authoritativeProjectionTimeout)
		_, err = session.reconcileCompletedDelegateChildren(projectionCtx)
		cancel()
	}
	session.stopReconciliation()
	if err == nil {
		err = session.engine.Close()
	}
	if err == nil {
		err = session.publishResult(result)
	}
	if err != nil {
		session.publishProjectionFailure(err)
	}
	session.finish()
}

func (session *interactionSession) publishResult(result agent.Result) error {
	member := runs.ExecutorMember{MemberID: result.ProcessID().String()}
	if result.Status() == agent.StatusCompleted {
		erased, ok := result.Output()
		if !ok {
			return errors.New("agentexec: completed Interaction has no output")
		}
		output, err := erased.Decode[interaction.Output]()
		if err != nil {
			return fmt.Errorf("decode Interaction output: %w", err)
		}
		if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil {
			return fmt.Errorf("unsupported Interaction completion source %q", output.Source)
		}
		modelOutput := output.ModelResponse.Output
		if modelOutput == nil || modelOutput.Message == nil {
			return errors.New("agentexec: Interaction output has no assistant message")
		}
		if !session.lifetime.send(runs.ExecutorEvent{
			Member: member, Payload: runs.AssistantMessageCompleted{Message: modelOutput.Message.Clone()},
		}) {
			return nil
		}
		session.maintainCompletedRoot()
	}
	end := session.segmentEnd(result)
	session.lifetime.send(runs.ExecutorEvent{Member: member, Payload: end})
	if session.lifecycleHooks != nil {
		session.lifecycleHooks.NotifyStopped(
			session.lifetime.context, session.start.SessionID, session.start.CWD, string(end.Reason),
		)
	}
	return nil
}

func (session *interactionSession) publishProjectionFailure(cause error) {
	member := runs.ExecutorMember{}
	session.state.mu.Lock()
	if session.state.admittedProcessID.Valid() {
		member.MemberID = session.state.admittedProcessID.String()
	}
	session.state.mu.Unlock()
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: executorDiagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	session.lifetime.send(runs.ExecutorEvent{
		Member:  member,
		Payload: runs.SegmentEnded{Reason: run.OutcomeFailed, Failure: &failure},
	})
}

func (session *interactionSession) release(ctx context.Context) error {
	session.lifetime.beginRelease()
	if err := session.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("agentexec: discard prepared waiting subtree before release: %w", err)
	}
	session.state.mu.Lock()
	process := session.state.process
	begun := session.state.begun
	finished := session.state.finished
	session.state.mu.Unlock()
	if !begun {
		session.failStart()
		return session.engine.Close()
	}
	if process != nil && !finished {
		if err := process.Kill(ctx, interactionReleaseReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
			return fmt.Errorf("agentexec: kill Interaction execution: %w", err)
		}
	}
	select {
	case <-session.lifetime.done:
		session.lifetime.workers.Wait()
		return session.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (session *interactionSession) segmentEnd(result agent.Result) runs.SegmentEnded {
	termination := result.Termination()
	end := segmentEndFromTermination(
		termination,
		session.segmentClock.duration(result.StartedAt(), result.FinishedAt()),
	)
	end.Usage = session.accounting.segmentUsage(result.ProcessID())
	return end
}

func segmentEndFromTermination(termination agent.Termination, duration time.Duration) runs.SegmentEnded {
	end := runs.SegmentEnded{Duration: duration}
	switch termination.Cause() {
	case agent.TerminationCauseCompletion:
		end.Reason = run.OutcomeCompleted
	case agent.TerminationCauseProcessDeadline,
		agent.TerminationCauseParentDeadline,
		agent.TerminationCauseHostDeadline:
		end.Reason = run.OutcomeTimedOut
		failure := run.Failure{
			Kind:   run.FailureTimeout,
			Detail: "executor deadline reached",
		}
		end.Failure = &failure
	case agent.TerminationCauseParentCancellation, agent.TerminationCauseHostCancellation:
		end.Reason = run.OutcomeCanceled
	case agent.TerminationCauseExecutionFailure:
		failure, _ := termination.Failure()
		if failure.Code() == "interaction.limit.model_calls" {
			end.Reason = run.OutcomeMaxSteps
			break
		}
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureAgentStuck,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.Failure = &problem
	case agent.TerminationCauseExternalFailure:
		end.Reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		if failure.Code() == "interaction.host.failed" {
			problem := run.Failure{
				Kind:   run.FailureInternal,
				Detail: executorDiagnostic(errors.New(failure.Message())),
			}
			end.Failure = &problem
			break
		}
		detail := executorDiagnostic(errors.New(failure.Message()))
		if detail == "" {
			detail = "model provider failed"
		}
		problem := run.Failure{
			Kind:   run.FailureProviderUnavailable,
			Detail: detail,
		}
		end.Failure = &problem
	case agent.TerminationCauseContractFailure, agent.TerminationCausePanic:
		end.Reason = run.OutcomeFailed
		failure, _ := termination.Failure()
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: executorDiagnostic(errors.New(failure.Message())),
		}
		end.Failure = &problem
	case agent.TerminationCauseEngineKill:
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: termination.Reason(),
		}
		end.Failure = &problem
	default:
		end.Reason = run.OutcomeFailed
		problem := run.Failure{
			Kind:   run.FailureInternal,
			Detail: "executor returned an unknown terminal cause",
		}
		end.Failure = &problem
	}
	return end
}
