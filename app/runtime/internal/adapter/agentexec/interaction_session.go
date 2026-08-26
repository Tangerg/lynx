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
func (i *interactionSession) beginDispatch(
	ctx context.Context,
	request agent.EffectRequest,
) (context.Context, func()) {
	bound, cancel := context.WithCancelCause(ctx)
	stopLifetimeBinding := context.AfterFunc(i.lifetime.context, func() {
		cancel(context.Cause(i.lifetime.context))
	})
	key := interactionDispatchKey(request)
	i.state.mu.Lock()
	if i.state.rootCancellationRequested || i.inCanceledSubtreeLocked(request.ProcessID()) {
		cancel(errInteractionRunCanceled)
	} else {
		i.state.activeDispatches[key] = activeInteractionDispatch{
			processID: request.ProcessID(),
			cancel:    cancel,
		}
	}
	i.state.mu.Unlock()
	return bound, func() {
		i.state.mu.Lock()
		delete(i.state.activeDispatches, key)
		i.state.mu.Unlock()
		stopLifetimeBinding()
		cancel(nil)
	}
}

func (i *interactionSession) cancelAllDispatches() {
	i.state.mu.Lock()
	i.state.rootCancellationRequested = true
	cancels := make([]context.CancelCauseFunc, 0, len(i.state.activeDispatches))
	for _, dispatch := range i.state.activeDispatches {
		cancels = append(cancels, dispatch.cancel)
	}
	i.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (i *interactionSession) cancelSubtreeDispatches(rootID agent.ProcessID) {
	i.state.mu.Lock()
	i.state.canceledSubtreeRoots[rootID] = struct{}{}
	cancels := make([]context.CancelCauseFunc, 0, len(i.state.activeDispatches))
	for _, dispatch := range i.state.activeDispatches {
		if i.inSubtreeLocked(dispatch.processID, rootID) {
			cancels = append(cancels, dispatch.cancel)
		}
	}
	i.state.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (i *interactionSession) inCanceledSubtreeLocked(processID agent.ProcessID) bool {
	for rootID := range i.state.canceledSubtreeRoots {
		if i.inSubtreeLocked(processID, rootID) {
			return true
		}
	}
	return false
}

func (i *interactionSession) inSubtreeLocked(
	processID agent.ProcessID,
	rootID agent.ProcessID,
) bool {
	for range len(i.state.delegateChildren) + 1 {
		if processID == rootID {
			return true
		}
		managed := i.state.delegateChildren[processID]
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

func (i *interactionState) attachObserver() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.observerWasAttached || i.finished {
		return false
	}
	if i.begun && i.boundary != interactionBoundaryContinuationStaged &&
		i.boundary != interactionBoundarySubtreePrepared {
		return false
	}
	i.observerWasAttached = true
	return true
}

func (i *interactionState) detachObserver() {
	i.mu.Lock()
	i.observerWasAttached = false
	i.mu.Unlock()
}

func (i *interactionState) observerAttached() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.observerWasAttached
}

func (i *interactionState) begin() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.begun || i.finished {
		return false
	}
	i.begun = true
	return true
}

func (i *interactionState) setProcess(process *agent.Process) {
	i.mu.Lock()
	i.process = process
	i.mu.Unlock()
}

func (i *interactionState) processHandle() *agent.Process {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.process
}

func (i *interactionSession) submitSteer(
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
	process := i.state.processHandle()
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
	i.state.mu.Lock()
	i.state.pendingSteers[signalID] = pendingInteractionSteer{
		content: transcript.CloneContent(content),
	}
	i.state.mu.Unlock()
	accepted, deliverErr := process.DeliverSignal(
		runExecutionContext(ctx, i.scope, i.start), signal,
	)
	if deliverErr != nil {
		// A context error only reports that the caller stopped waiting. Engine may
		// already have accepted the command, so retain its exact product mapping
		// until ModelInvocation attributes it or the session is released.
		if !errors.Is(deliverErr, context.Canceled) &&
			!errors.Is(deliverErr, context.DeadlineExceeded) {
			i.removePendingSteer(signalID)
		}
		return fmt.Errorf("agentexec: deliver Interaction steer Signal: %w", deliverErr)
	}
	if !accepted {
		i.removePendingSteer(signalID)
		return errors.New("agentexec: Interaction steer Signal was not accepted")
	}
	return nil
}

func (i *interactionSession) removePendingSteer(signalID agent.SignalID) {
	i.state.mu.Lock()
	delete(i.state.pendingSteers, signalID)
	i.state.mu.Unlock()
}

func (i *interactionSession) commitAppliedSteers(
	ctx context.Context,
	member runs.ExecutorMember,
	signalIDs []agent.SignalID,
) error {
	if len(signalIDs) == 0 {
		return nil
	}
	i.state.mu.Lock()
	messages := make([][]transcript.ContentBlock, len(signalIDs))
	seen := make(map[agent.SignalID]struct{}, len(signalIDs))
	for index, signalID := range signalIDs {
		if _, duplicate := seen[signalID]; duplicate {
			i.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution repeats steer Signal %s", signalID)
		}
		seen[signalID] = struct{}{}
		pending, found := i.state.pendingSteers[signalID]
		if !found {
			i.state.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution names unknown steer Signal %s", signalID)
		}
		messages[index] = transcript.CloneContent(pending.content)
	}
	i.state.mu.Unlock()
	if err := i.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
		return fmt.Errorf("agentexec: commit applied Interaction steers: %w", err)
	}
	i.state.mu.Lock()
	for _, signalID := range signalIDs {
		delete(i.state.pendingSteers, signalID)
	}
	i.state.mu.Unlock()
	return nil
}

func (i *interactionSession) startWorkers() {
	i.lifetime.workers.Add(1)
	go func() {
		defer i.lifetime.workers.Done()
		i.await()
	}()
	i.lifetime.reconcilers.Add(2)
	go func() {
		defer i.lifetime.reconcilers.Done()
		i.reconcileUnknownEffects()
	}()
	go func() {
		defer i.lifetime.reconcilers.Done()
		i.reconcileExecutionState()
	}()
}

func (i *interactionSession) failStart() {
	i.finish()
}

func (i *interactionSession) stopReconciliation() {
	i.lifetime.stop()
	i.lifetime.reconcilers.Wait()
}

func (i *interactionSession) finish() {
	i.lifetime.finishOnce.Do(func() {
		i.state.mu.Lock()
		i.state.finished = true
		i.state.mu.Unlock()
		i.stopReconciliation()
		close(i.lifetime.events)
		close(i.lifetime.done)
	})
}

func (i *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
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
		member, found := i.executorMemberByProcessID(delta.ProcessID())
		if found && i.lifetime.offer(runs.ExecutorEvent{Member: member, Payload: payload}) {
			continue
		}
		trace.SpanFromContext(ctx).AddEvent(
			"agentexec.delta.dropped",
			trace.WithAttributes(attribute.String("process.id", delta.ProcessID().String())),
		)
	}
}

func (i *interactionSession) flushDeltas(ctx context.Context) error {
	if i.engine == nil {
		return errors.New("agentexec: Interaction engine is unavailable")
	}
	if err := i.engine.FlushDeltas(ctx); err != nil {
		return fmt.Errorf("agentexec: flush model deltas: %w", err)
	}
	return nil
}

func (i *interactionSession) observeFrameworkEvent(_ context.Context, event agent.Event) {
	if event.Relation().RootID() != i.processRootID() {
		return
	}
	i.lifetime.wakeState()
}

func (i *interactionSession) processRootID() agent.ProcessID {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.process == nil {
		return agent.ProcessID{}
	}
	return i.state.process.Relation().RootID()
}

func (i *interactionSession) commitFact(
	ctx context.Context,
	member runs.ExecutorMember,
	fact runs.ExecutionFact,
) error {
	ctx, cancel := i.lifetime.bind(ctx)
	defer cancel()
	commit, receipt, err := runs.NewExecutionFactCommit(fact)
	if err != nil {
		return err
	}
	event := runs.ExecutorEvent{Member: member, Payload: commit}
	if err := i.lifetime.sendAuthoritative(ctx, event); err != nil {
		return err
	}
	return receipt.Await(ctx)
}

func (i *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(i.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.unknownWake:
		case <-ticker.C:
		case <-i.lifetime.context.Done():
			return
		}
		if i.reportUnknownEffects() {
			return
		}
	}
}

func (i *interactionSession) reconcileExecutionState() {
	ticker := time.NewTicker(i.statePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-i.lifetime.stateWake:
		case <-ticker.C:
		case <-i.lifetime.context.Done():
			return
		}
		ctx, cancel := context.WithTimeout(i.lifetime.context, authoritativeProjectionTimeout)
		progressed, err := i.reconcileCompletedDelegateChildren(ctx)
		cancel()
		if err != nil {
			i.publishProjectionFailure(err)
			return
		}
		if progressed {
			continue
		}
		if i.publishWaitingBoundary() {
			continue
		}
	}
}

func (i *interactionSession) publishWaitingBoundary() bool {
	i.state.mu.Lock()
	process := i.state.process
	if process == nil || i.state.finished || i.state.boundary != interactionBoundaryInactive {
		i.state.mu.Unlock()
		return false
	}
	i.state.mu.Unlock()
	if process.Status() != agent.StatusWaiting {
		return false
	}
	ctx, cancel := context.WithTimeout(i.lifetime.context, authoritativeProjectionTimeout)
	defer cancel()
	snapshot, interruptions, found, err := i.captureHumanInputBarrier(ctx)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	if !found {
		return false
	}
	checkpoint, err := i.executorCheckpoint(snapshot)
	if err != nil {
		i.publishProjectionFailure(err)
		return false
	}
	i.state.mu.Lock()
	if i.state.finished || i.state.boundary != interactionBoundaryInactive ||
		i.state.process != process || process.Status() != agent.StatusWaiting {
		i.state.mu.Unlock()
		return false
	}
	i.state.boundary = interactionBoundaryWaiting
	i.state.waitingCheckpoint = checkpoint.Clone()
	i.state.mu.Unlock()
	published := i.lifetime.send(runs.ExecutorEvent{
		Member: i.executorMember(process.Relation()),
		Payload: runs.TreeInterrupted{
			Checkpoint: checkpoint, Interruptions: interruptions,
		},
	})
	if published && i.lifecycleHooks != nil {
		i.lifecycleHooks.NotifyWaiting(
			i.lifetime.context, i.start.SessionID, i.start.CWD,
		)
	}
	return published
}

func (i *interactionSession) stageContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryWaiting || i.state.observerWasAttached ||
		!isInteractionWaitingBoundary(i.state.process.Status()) {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(i.state.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	i.state.boundary = interactionBoundaryContinuationStaged
	return nil
}

func (i *interactionSession) beginContinuation(allowedInterrupts []interrupt.Kind) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.process == nil {
		return runs.ErrExecutorNotLive
	}
	if i.state.boundary != interactionBoundaryContinuationStaged || !i.state.observerWasAttached ||
		!isInteractionWaitingBoundary(i.state.process.Status()) {
		return errors.New("agentexec: Interaction continuation was not staged and observed")
	}
	if !slices.Equal(i.start.InterruptKinds, allowedInterrupts) {
		return errors.New("agentexec: continuation capabilities differ from the staged Interaction")
	}
	return nil
}

func isInteractionWaitingBoundary(status agent.Status) bool {
	return status == agent.StatusWaiting || status == agent.StatusPaused
}

func (i *interactionSession) continuationAccepted() {
	i.state.mu.Lock()
	i.state.boundary = interactionBoundaryInactive
	i.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
	i.state.mu.Unlock()
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.Scope == right.Scope && left.ModelSelection == right.ModelSelection &&
		left.Limits == right.Limits && slices.Equal(left.Usage.Models, right.Usage.Models) &&
		bytes.Equal(left.Payload, right.Payload)
}

func (i *interactionSession) reportUnknownEffects() bool {
	ctx, cancel := context.WithTimeout(i.lifetime.context, authoritativeProjectionTimeout)
	defer cancel()
	ids, err := i.unknownEffectIDs(ctx)
	if err != nil || len(ids) == 0 {
		return false
	}
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = id.String()
	}
	slices.Sort(values)
	values = slices.Compact(values)
	i.state.mu.Lock()
	if i.state.unknownReported {
		i.state.mu.Unlock()
		return true
	}
	i.state.unknownReported = true
	member := runs.ExecutorMember{MemberID: i.state.process.Relation().ProcessID().String()}
	i.state.mu.Unlock()
	return i.lifetime.send(runs.ExecutorEvent{
		Member: member, Payload: runs.UnknownEffectsDetected{IDs: values},
	})
}

func (i *interactionSession) await() {
	joinCtx := context.WithoutCancel(i.lifetime.context)
	result, err := i.state.process.Await(joinCtx)
	if err == nil {
		projectionCtx, cancel := context.WithTimeout(joinCtx, authoritativeProjectionTimeout)
		_, err = i.reconcileCompletedDelegateChildren(projectionCtx)
		cancel()
	}
	i.stopReconciliation()
	if err == nil {
		err = i.engine.Close()
	}
	if err == nil {
		err = i.publishResult(result)
	}
	if err != nil {
		i.publishProjectionFailure(err)
	}
	i.finish()
}

func (i *interactionSession) publishResult(result agent.Result) error {
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
		if !i.lifetime.send(runs.ExecutorEvent{
			Member: member, Payload: runs.AssistantMessageCompleted{Message: modelOutput.Message.Clone()},
		}) {
			return nil
		}
		i.maintainCompletedRoot()
	}
	end := i.segmentEnd(result)
	i.lifetime.send(runs.ExecutorEvent{Member: member, Payload: end})
	if i.lifecycleHooks != nil {
		i.lifecycleHooks.NotifyStopped(
			i.lifetime.context, i.start.SessionID, i.start.CWD, string(end.Reason),
		)
	}
	return nil
}

func (i *interactionSession) publishProjectionFailure(cause error) {
	member := runs.ExecutorMember{}
	i.state.mu.Lock()
	if i.state.admittedProcessID.Valid() {
		member.MemberID = i.state.admittedProcessID.String()
	}
	i.state.mu.Unlock()
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: executorDiagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	i.lifetime.send(runs.ExecutorEvent{
		Member:  member,
		Payload: runs.SegmentEnded{Reason: run.OutcomeFailed, Failure: &failure},
	})
}

func (i *interactionSession) release(ctx context.Context) error {
	i.lifetime.beginRelease()
	if err := i.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("agentexec: discard prepared waiting subtree before release: %w", err)
	}
	i.state.mu.Lock()
	process := i.state.process
	begun := i.state.begun
	finished := i.state.finished
	i.state.mu.Unlock()
	if !begun {
		i.failStart()
		return i.engine.Close()
	}
	if process != nil && !finished {
		if err := process.Kill(ctx, interactionReleaseReason); err != nil && !errors.Is(err, agent.ErrProcessFinished) {
			return fmt.Errorf("agentexec: kill Interaction execution: %w", err)
		}
	}
	select {
	case <-i.lifetime.done:
		i.lifetime.workers.Wait()
		return i.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *interactionSession) segmentEnd(result agent.Result) runs.SegmentEnded {
	termination := result.Termination()
	end := segmentEndFromTermination(
		termination,
		i.segmentClock.duration(result.StartedAt(), result.FinishedAt()),
	)
	end.Usage = i.accounting.segmentUsage(result.ProcessID())
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
