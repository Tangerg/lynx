package agentexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type interactionSession struct {
	ref           runs.ExecutorRef
	scope         runs.ExecutionScope
	deployment    agent.Deployment
	input         agent.Input
	engine        *agent.Engine
	events        chan runs.ExecutorEvent
	done          chan struct{}
	releasing     chan struct{}
	lifecycle     context.Context
	stopLifecycle context.CancelFunc
	unknownWake   chan struct{}
	stateWake     chan struct{}

	mu                        sync.Mutex
	childProjectionMu         sync.Mutex
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
	releaseOnce               sync.Once
	finishOnce                sync.Once
	workers                   sync.WaitGroup
	reconcilers               sync.WaitGroup
	usageByProcess            map[agent.ProcessID]map[string]accounting.ModelUsage
	carriedUsage              map[string]accounting.ModelUsage
	provider                  string
	fallbackModel             string
	pricing                   accounting.Pricing
	unknownPollInterval       time.Duration
	statePollInterval         time.Duration
	mcpToolAutoApproved       func(server, tool string) bool
	maintenance               RunMaintenance
	lifecycleHooks            InteractionLifecycleHooks
	buildID                   string
	start                     runs.RootExecutionStart
	unknownReported           bool
	toolOutcomeKey            string
	toolOutcomeDigest         [sha256.Size]byte
	toolOutcomeRepeats        int
	toolCalls                 int
	deployments               *interactionDeploymentSet
	delegateCalls             map[delegateCallIdentity]*managedDelegateCall
	delegateChildren          map[agent.ProcessID]*managedDelegateCall
	committedModelReplies     map[agent.ProcessID]corechat.Message
	activeDispatches          map[string]activeInteractionDispatch
	canceledSubtreeRoots      map[agent.ProcessID]struct{}
	rootCancellationRequested bool
	segmentStartedAt          time.Time
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

func (session *interactionSession) repeatedToolOutcome(toolName string, arguments tool.Arguments) int {
	key := toolName + "\x00" + arguments.Canonical()
	session.mu.Lock()
	defer session.mu.Unlock()
	if key != session.toolOutcomeKey {
		return 0
	}
	return session.toolOutcomeRepeats
}

func (session *interactionSession) resetRepeatedToolOutcome() {
	session.mu.Lock()
	session.toolOutcomeRepeats = 0
	session.mu.Unlock()
}

func (session *interactionSession) recordToolOutcome(
	toolName string,
	arguments tool.Arguments,
	result string,
) {
	key := toolName + "\x00" + arguments.Canonical()
	digest := sha256.Sum256([]byte(result))
	session.mu.Lock()
	defer session.mu.Unlock()
	if key == session.toolOutcomeKey && digest == session.toolOutcomeDigest {
		session.toolOutcomeRepeats++
		return
	}
	session.toolOutcomeKey = key
	session.toolOutcomeDigest = digest
	session.toolOutcomeRepeats = 1
}

func newInteractionSession(
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
	config InteractionExecutorConfig,
) *interactionSession {
	lifecycle, stopLifecycle := context.WithCancel(context.Background())
	provider := start.ModelSelection.Provider()
	if provider == "" {
		provider = config.Provider
	}
	return &interactionSession{
		ref: ref, scope: rootExecutionScope(start), events: make(chan runs.ExecutorEvent, interactionEventBuffer),
		done: make(chan struct{}), releasing: make(chan struct{}), lifecycle: lifecycle, stopLifecycle: stopLifecycle,
		unknownWake: make(chan struct{}, 1), usageByProcess: make(map[agent.ProcessID]map[string]accounting.ModelUsage),
		carriedUsage:          make(map[string]accounting.ModelUsage),
		pendingSteers:         make(map[agent.SignalID]pendingInteractionSteer),
		delegateCalls:         make(map[delegateCallIdentity]*managedDelegateCall),
		delegateChildren:      make(map[agent.ProcessID]*managedDelegateCall),
		committedModelReplies: make(map[agent.ProcessID]corechat.Message),
		activeDispatches:      make(map[string]activeInteractionDispatch),
		canceledSubtreeRoots:  make(map[agent.ProcessID]struct{}),
		stateWake:             make(chan struct{}, 1),
		provider:              provider, fallbackModel: start.ModelSelection.Model(), pricing: config.Pricing,
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
	key := interactionDispatchKey(request)
	session.mu.Lock()
	if session.rootCancellationRequested || session.inCanceledSubtreeLocked(request.ProcessID()) {
		cancel(errInteractionRunCanceled)
	} else {
		session.activeDispatches[key] = activeInteractionDispatch{
			processID: request.ProcessID(),
			cancel:    cancel,
		}
	}
	session.mu.Unlock()
	return bound, func() {
		session.mu.Lock()
		delete(session.activeDispatches, key)
		session.mu.Unlock()
		cancel(nil)
	}
}

func (session *interactionSession) cancelAllDispatches() {
	session.mu.Lock()
	session.rootCancellationRequested = true
	cancels := make([]context.CancelCauseFunc, 0, len(session.activeDispatches))
	for _, dispatch := range session.activeDispatches {
		cancels = append(cancels, dispatch.cancel)
	}
	session.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (session *interactionSession) cancelSubtreeDispatches(rootID agent.ProcessID) {
	session.mu.Lock()
	session.canceledSubtreeRoots[rootID] = struct{}{}
	cancels := make([]context.CancelCauseFunc, 0, len(session.activeDispatches))
	for _, dispatch := range session.activeDispatches {
		if session.inSubtreeLocked(dispatch.processID, rootID) {
			cancels = append(cancels, dispatch.cancel)
		}
	}
	session.mu.Unlock()
	for _, cancel := range cancels {
		cancel(errInteractionRunCanceled)
	}
}

func (session *interactionSession) inCanceledSubtreeLocked(processID agent.ProcessID) bool {
	for rootID := range session.canceledSubtreeRoots {
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
	for range len(session.delegateChildren) + 1 {
		if processID == rootID {
			return true
		}
		managed := session.delegateChildren[processID]
		if managed == nil {
			return false
		}
		processID = managed.identity.parentID
	}
	return false
}

// startSegment records the executor-side start of the product Segment that is
// about to advance the already-live Process tree. A Process may survive several
// human-input boundaries, so its framework StartedAt cannot represent a resumed
// product Segment.
func (session *interactionSession) startSegment() {
	startedAt := time.Now().UTC()
	session.mu.Lock()
	session.segmentStartedAt = startedAt
	session.mu.Unlock()
}

func (session *interactionSession) resultSegmentDuration(result agent.Result) time.Duration {
	session.mu.Lock()
	segmentStartedAt := session.segmentStartedAt
	session.mu.Unlock()
	return interactionSegmentDuration(result.StartedAt(), segmentStartedAt, result.FinishedAt())
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

func (session *interactionSession) recordToolCall() {
	session.mu.Lock()
	session.toolCalls++
	session.mu.Unlock()
}

// recordCommittedModelReply retains the exact assistant value accepted by the
// authoritative Run projection. A delegated Process has to close that same
// value at its semantic completion boundary; reconstructing it from the
// delegate's text result would discard structured parts and metadata.
func (session *interactionSession) recordCommittedModelReply(
	processID agent.ProcessID,
	message corechat.Message,
) {
	cloned := message.Clone()
	session.mu.Lock()
	session.committedModelReplies[processID] = cloned
	session.mu.Unlock()
}

func (session *interactionSession) committedModelReplyFor(
	processID agent.ProcessID,
) (corechat.Message, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	message, found := session.committedModelReplies[processID]
	return message.Clone(), found
}

func (session *interactionSession) forgetCommittedModelReply(processID agent.ProcessID) {
	session.mu.Lock()
	delete(session.committedModelReplies, processID)
	session.mu.Unlock()
}

func (session *interactionSession) attachObserver() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.observerWasAttached || session.finished {
		return false
	}
	if session.begun && session.boundary != interactionBoundaryContinuationStaged &&
		session.boundary != interactionBoundarySubtreePrepared {
		return false
	}
	session.observerWasAttached = true
	return true
}

func (session *interactionSession) detachObserver() {
	session.mu.Lock()
	session.observerWasAttached = false
	session.mu.Unlock()
}

func (session *interactionSession) observerAttached() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.observerWasAttached
}

func (session *interactionSession) begin() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.begun || session.finished {
		return false
	}
	session.begun = true
	return true
}

func (session *interactionSession) setProcess(process *agent.Process) {
	session.mu.Lock()
	session.process = process
	session.mu.Unlock()
}

func (session *interactionSession) processHandle() *agent.Process {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.process
}

func (session *interactionSession) submitSteer(
	ctx context.Context,
	message corechat.Message,
	content []transcript.ContentBlock,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	process := session.processHandle()
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
	session.mu.Lock()
	session.pendingSteers[signalID] = pendingInteractionSteer{
		content: transcript.CloneContent(content),
	}
	session.mu.Unlock()
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
	session.mu.Lock()
	delete(session.pendingSteers, signalID)
	session.mu.Unlock()
}

func (session *interactionSession) commitAppliedSteers(
	ctx context.Context,
	member runs.ExecutorMember,
	signalIDs []agent.SignalID,
) error {
	if len(signalIDs) == 0 {
		return nil
	}
	session.mu.Lock()
	messages := make([][]transcript.ContentBlock, len(signalIDs))
	seen := make(map[agent.SignalID]struct{}, len(signalIDs))
	for index, signalID := range signalIDs {
		if _, duplicate := seen[signalID]; duplicate {
			session.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution repeats steer Signal %s", signalID)
		}
		seen[signalID] = struct{}{}
		pending, found := session.pendingSteers[signalID]
		if !found {
			session.mu.Unlock()
			return fmt.Errorf("agentexec: model attribution names unknown steer Signal %s", signalID)
		}
		messages[index] = transcript.CloneContent(pending.content)
	}
	session.mu.Unlock()
	if err := session.commitFact(ctx, member, runs.SteerMessagesApplied{Messages: messages}); err != nil {
		return fmt.Errorf("agentexec: commit applied Interaction steers: %w", err)
	}
	session.mu.Lock()
	for _, signalID := range signalIDs {
		delete(session.pendingSteers, signalID)
	}
	session.mu.Unlock()
	return nil
}

func (session *interactionSession) startWorkers() {
	session.workers.Add(1)
	go func() {
		defer session.workers.Done()
		session.await()
	}()
	session.reconcilers.Add(2)
	go func() {
		defer session.reconcilers.Done()
		session.reconcileUnknownEffects()
	}()
	go func() {
		defer session.reconcilers.Done()
		session.reconcileExecutionState()
	}()
}

func (session *interactionSession) failStart() {
	session.finish()
}

func (session *interactionSession) stopReconciliation() {
	session.stopLifecycle()
	session.reconcilers.Wait()
}

func (session *interactionSession) finish() {
	session.finishOnce.Do(func() {
		session.mu.Lock()
		session.finished = true
		session.mu.Unlock()
		session.stopReconciliation()
		close(session.events)
		close(session.done)
	})
}

func (session *interactionSession) projectDelta(ctx context.Context, delta agent.Delta) {
	parsed, err := interaction.ParseModelResponseDelta(delta.Payload())
	if err != nil {
		return
	}
	response := parsed.Response()
	for _, choice := range response.Choices {
		if choice.Message == nil {
			continue
		}
		for _, part := range choice.Message.Parts {
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
			if found && session.offer(runs.ExecutorEvent{Member: member, Payload: payload}) {
				continue
			}
			trace.SpanFromContext(ctx).AddEvent(
				"agentexec.delta.dropped",
				trace.WithAttributes(attribute.String("process.id", delta.ProcessID().String())),
			)
		}
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
	select {
	case session.stateWake <- struct{}{}:
	default:
	}
}

func (session *interactionSession) processRootID() agent.ProcessID {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.process == nil {
		return agent.ProcessID{}
	}
	return session.process.Relation().RootID()
}

func (session *interactionSession) offer(event runs.ExecutorEvent) bool {
	select {
	case session.events <- event:
		return true
	default:
		return false
	}
}

func (session *interactionSession) commitFact(
	ctx context.Context,
	member runs.ExecutorMember,
	fact runs.ExecutionFact,
) error {
	ctx, cancel := session.lifecycleContext(ctx)
	defer cancel()
	commit, receipt, err := runs.NewExecutionFactCommit(fact)
	if err != nil {
		return err
	}
	event := runs.ExecutorEvent{Member: member, Payload: commit}
	select {
	case session.events <- event:
	case <-session.releasing:
		return errors.New("agentexec: execution released before authoritative fact commit")
	case <-ctx.Done():
		return ctx.Err()
	}
	return receipt.Await(ctx)
}

func (session *interactionSession) lifecycleContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(session.lifecycle, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

func (session *interactionSession) wakeUnknownReconciliation() {
	select {
	case session.unknownWake <- struct{}{}:
	default:
	}
}

func (session *interactionSession) reconcileUnknownEffects() {
	ticker := time.NewTicker(session.unknownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-session.unknownWake:
		case <-ticker.C:
		case <-session.lifecycle.Done():
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
		case <-session.stateWake:
		case <-ticker.C:
		case <-session.lifecycle.Done():
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), authoritativeProjectionTimeout)
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
	session.mu.Lock()
	process := session.process
	if process == nil || session.finished || session.boundary != interactionBoundaryInactive {
		session.mu.Unlock()
		return false
	}
	session.mu.Unlock()
	if process.Status() != agent.StatusWaiting {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), authoritativeProjectionTimeout)
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
	session.mu.Lock()
	if session.finished || session.boundary != interactionBoundaryInactive ||
		session.process != process || process.Status() != agent.StatusWaiting {
		session.mu.Unlock()
		return false
	}
	session.boundary = interactionBoundaryWaiting
	session.waitingCheckpoint = checkpoint.Clone()
	session.mu.Unlock()
	published := session.send(runs.ExecutorEvent{
		Member: session.executorMember(process.Relation()),
		Payload: runs.TreeInterrupted{
			Checkpoint: checkpoint, Interruptions: interruptions,
		},
	})
	if published && session.lifecycleHooks != nil {
		session.lifecycleHooks.NotifyWaiting(
			session.lifecycle, session.start.SessionID, session.start.CWD,
		)
	}
	return published
}

func (session *interactionSession) stageContinuation(checkpoint runs.ExecutorCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finished || session.process == nil {
		return runs.ErrExecutorNotLive
	}
	if session.boundary != interactionBoundaryWaiting || session.observerWasAttached ||
		!isInteractionWaitingBoundary(session.process.Status()) {
		return runs.ErrExecutionClaimed
	}
	if !executorCheckpointsEqual(session.waitingCheckpoint, checkpoint) {
		return fmt.Errorf("%w: live Interaction checkpoint differs from the claimed waiting boundary", runs.ErrInvalidExecutorCheckpoint)
	}
	session.boundary = interactionBoundaryContinuationStaged
	return nil
}

func (session *interactionSession) beginContinuation(allowedInterrupts []interrupt.Kind) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.finished || session.process == nil {
		return runs.ErrExecutorNotLive
	}
	if session.boundary != interactionBoundaryContinuationStaged || !session.observerWasAttached ||
		!isInteractionWaitingBoundary(session.process.Status()) {
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
	session.mu.Lock()
	session.boundary = interactionBoundaryInactive
	session.waitingCheckpoint = runs.ExecutorCheckpoint{}
	session.mu.Unlock()
}

func executorCheckpointsEqual(left, right runs.ExecutorCheckpoint) bool {
	return left.RootMemberID == right.RootMemberID && left.BuildID == right.BuildID &&
		left.Scope == right.Scope && left.ModelSelection == right.ModelSelection &&
		left.Limits == right.Limits && slices.Equal(left.Usage.Models, right.Usage.Models) &&
		bytes.Equal(left.Payload, right.Payload)
}

func (session *interactionSession) accountingSnapshot() accounting.Snapshot {
	session.mu.Lock()
	defer session.mu.Unlock()
	byModel := make(map[string]accounting.ModelUsage)
	mergeInteractionUsage(byModel, session.carriedUsage)
	for _, processUsage := range session.usageByProcess {
		mergeInteractionUsage(byModel, processUsage)
	}
	models := make([]accounting.ModelUsage, 0, len(byModel))
	for _, usage := range byModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	return accounting.Snapshot{Models: models}
}

func mergeInteractionUsage(
	target map[string]accounting.ModelUsage,
	source map[string]accounting.ModelUsage,
) {
	for model, usage := range source {
		current := target[model]
		if current.Model == "" {
			current.Model = model
		}
		current.Add(usage.TokenUsage)
		current.CostUSD += usage.CostUSD
		current.Calls += usage.Calls
		target[model] = current
	}
}

func (session *interactionSession) interactionCheckpointPayload(
	tree agent.TreeSnapshot,
) ([]byte, error) {
	session.mu.Lock()
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(session.usageByProcess))
	for processID, byModel := range session.usageByProcess {
		usageByProcess[processID] = maps.Clone(byModel)
	}
	carried := maps.Clone(session.carriedUsage)
	pendingSteers := make(map[agent.SignalID]pendingInteractionSteer, len(session.pendingSteers))
	for signalID, pending := range session.pendingSteers {
		pendingSteers[signalID] = pendingInteractionSteer{content: transcript.CloneContent(pending.content)}
	}
	instructions, err := interactionInstructionContext(session.start.WorkingContext)
	session.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return encodeInteractionCheckpointPayload(tree, usageByProcess, carried, instructions, pendingSteers)
}

func (session *interactionSession) reportUnknownEffects() bool {
	ctx, cancel := context.WithTimeout(context.Background(), authoritativeProjectionTimeout)
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
	session.mu.Lock()
	if session.unknownReported {
		session.mu.Unlock()
		return true
	}
	session.unknownReported = true
	member := runs.ExecutorMember{MemberID: session.process.Relation().ProcessID().String()}
	session.mu.Unlock()
	return session.send(runs.ExecutorEvent{
		Member: member, Payload: runs.UnknownEffectsDetected{IDs: values},
	})
}

func (session *interactionSession) accountModelCall(
	invocation interaction.ModelInvocation,
	callID string,
	response *corechat.Response,
) (runs.ModelCallCompleted, error) {
	delta := modelUsage(response, session.provider, session.fallbackModel, session.pricing)
	if err := delta.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: account model call: %w", err)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	processID := invocation.Relation().ProcessID()
	usageByModel := session.usageByProcess[processID]
	if usageByModel == nil {
		usageByModel = make(map[string]accounting.ModelUsage)
		session.usageByProcess[processID] = usageByModel
	}
	current := usageByModel[delta.Model]
	if current.Model == "" {
		current.Model = delta.Model
	}
	current.Add(delta.TokenUsage)
	current.CostUSD += delta.CostUSD
	current.Calls += delta.Calls
	if err := current.Validate(); err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: aggregate model call: %w", err)
	}
	usageByModel[delta.Model] = current
	models := make([]accounting.ModelUsage, 0, len(usageByModel))
	for _, usage := range usageByModel {
		models = append(models, usage)
	}
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	total, err := (accounting.Snapshot{Models: models}).Total()
	if err != nil {
		return runs.ModelCallCompleted{}, fmt.Errorf("agentexec: total model usage: %w", err)
	}
	if total.Calls != int(invocation.ModelCallSequence()) {
		return runs.ModelCallCompleted{}, fmt.Errorf(
			"agentexec: model call sequence %d differs from accounted calls %d",
			invocation.ModelCallSequence(), total.Calls,
		)
	}
	choice := response.First()
	if choice == nil || choice.Message == nil {
		return runs.ModelCallCompleted{}, errors.New("agentexec: account model call without an assistant message")
	}
	return runs.ModelCallCompleted{
		CallID: callID, Message: choice.Message.Clone(), TokenUsage: total.TokenUsage,
		ByModel: slices.Clone(models), CostUSD: total.CostUSD, Steps: total.Calls,
		ContextTokens: response.Usage.InputTokens,
	}, nil
}

func (session *interactionSession) await() {
	result, err := session.process.Await(context.Background())
	if err == nil {
		projectionCtx, cancel := context.WithTimeout(context.Background(), authoritativeProjectionTimeout)
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
		output, err := agent.DecodeOutput[interaction.Output](erased)
		if err != nil {
			return fmt.Errorf("decode Interaction output: %w", err)
		}
		if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil {
			return fmt.Errorf("unsupported Interaction completion source %q", output.Source)
		}
		choice := output.ModelResponse.First()
		if choice == nil || choice.Message == nil {
			return errors.New("agentexec: Interaction output has no assistant message")
		}
		if !session.send(runs.ExecutorEvent{
			Member: member, Payload: runs.AssistantMessageCompleted{Message: choice.Message.Clone()},
		}) {
			return nil
		}
		session.maintainCompletedRoot()
	}
	end := session.segmentEnd(result)
	session.send(runs.ExecutorEvent{Member: member, Payload: end})
	if session.lifecycleHooks != nil {
		session.lifecycleHooks.NotifyStopped(
			session.lifecycle, session.start.SessionID, session.start.CWD, string(end.Reason),
		)
	}
	return nil
}

func (session *interactionSession) publishProjectionFailure(cause error) {
	member := runs.ExecutorMember{}
	session.mu.Lock()
	if session.admittedProcessID.Valid() {
		member.MemberID = session.admittedProcessID.String()
	}
	session.mu.Unlock()
	failure := run.Failure{
		Kind:   run.FailureInternal,
		Detail: executorDiagnostic(cause),
	}
	if failure.Detail == "" {
		failure.Detail = "executor result could not be projected"
	}
	session.send(runs.ExecutorEvent{
		Member:  member,
		Payload: runs.SegmentEnded{Reason: run.OutcomeFailed, Failure: &failure},
	})
}

func (session *interactionSession) send(event runs.ExecutorEvent) bool {
	select {
	case session.events <- event:
		return true
	case <-session.releasing:
		return false
	}
}

func (session *interactionSession) release(ctx context.Context) error {
	session.releaseOnce.Do(func() {
		session.stopLifecycle()
		close(session.releasing)
	})
	if err := session.discardPreparedSubtree(ctx); err != nil {
		return fmt.Errorf("agentexec: discard prepared waiting subtree before release: %w", err)
	}
	session.mu.Lock()
	process := session.process
	begun := session.begun
	finished := session.finished
	session.mu.Unlock()
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
	case <-session.done:
		session.workers.Wait()
		return session.engine.Close()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (session *interactionSession) segmentEnd(result agent.Result) runs.SegmentEnded {
	termination := result.Termination()
	end := segmentEndFromTermination(termination, session.resultSegmentDuration(result))
	session.mu.Lock()
	usageByModel := session.usageByProcess[result.ProcessID()]
	models := make([]accounting.ModelUsage, 0, len(usageByModel))
	for _, usage := range usageByModel {
		models = append(models, usage)
	}
	session.mu.Unlock()
	slices.SortFunc(models, func(left, right accounting.ModelUsage) int {
		return strings.Compare(left.Model, right.Model)
	})
	if len(models) > 0 {
		if total, err := (accounting.Snapshot{Models: models}).Total(); err == nil {
			end.Usage = &runs.SegmentUsage{
				Tokens: total.TokenUsage, ByModel: models,
				CostUSD: total.CostUSD, Steps: total.Calls,
			}
		}
	}
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
