package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sync/atomic"
	"time"
)

// treeRuntime is the sole Framework state committer for one root Process tree.
// Strategy reduction and external dispatch run as jobs; only their fenced
// completions may re-enter this owner line.
type treeRuntime struct {
	engine      *Engine
	rootID      ProcessID
	context     context.Context
	commands    chan treeCommand
	completions chan treeJobCompletion
	processes   map[ProcessID]*processState
	childWaits  map[WaitID]*childWaitRegistration
	runnable    []ProcessID
	queued      map[ProcessID]struct{}
	jobs        map[ProcessID]*processJob
	freeze      *activeTreeFreeze
	done        chan struct{}
}

type treeCommandKind uint8

const (
	treeCommandInvalid treeCommandKind = iota
	treeCommandProcess
	treeCommandAcquireFreeze
	treeCommandReleaseFreeze
	treeCommandApplyFreeze
)

type treeCommand struct {
	kind        treeCommandKind
	processID   ProcessID
	process     processCommand
	freeze      *treeFreeze
	acquisition *treeFreezeAcquisition
	projection  *treeStateProjection
	response    chan error
}

func newTreeProcessCommand(processID ProcessID, command processCommand) treeCommand {
	return treeCommand{kind: treeCommandProcess, processID: processID, process: command}
}

type treeFreezeAcquisition struct {
	response chan treeFreezeAcquisitionResult
	canceled chan struct{}
	mode     treeFreezeMode
}

type treeFreezeMode uint8

const (
	treeFreezeModeInvalid treeFreezeMode = iota
	treeFreezeModeSnapshot
	treeFreezeModeExclusive
)

type treeFreezeAcquisitionResult struct {
	freeze   *treeFreeze
	snapshot TreeSnapshot
	err      error
}

type activeTreeFreeze struct {
	acquisition *treeFreezeAcquisition
	freeze      *treeFreeze
	deferred    []treeCommand
	ready       bool
}

type treeStateProjection struct {
	changes    []*preparedProcessStateChange
	childWaits []*childWaitRegistration
}

type processAttempt uint64

func (p processAttempt) valid() bool { return p != 0 }

type processJobKind uint8

const (
	processJobInvalid processJobKind = iota
	processJobStep
	processJobPreparedAcknowledgement
	processJobDispatch
	processJobChildStart
)

type processJob struct {
	kind       processJobKind
	attempt    processAttempt
	cancel     context.CancelFunc
	stale      bool
	effectID   EffectID
	childStart *childStartPlan
	startedAt  time.Time
}

type treeJobCompletion struct {
	processID       ProcessID
	attempt         processAttempt
	kind            processJobKind
	step            stepJobResult
	acknowledgement error
	dispatch        dispatchJobResult
	childStart      childStartJobResult
}

type stepJobResult struct {
	transition     Transition
	candidate      Execution
	candidateState ExecutionState
	stage          stepJobStage
	err            error
}

type stepJobStage uint8

const (
	stepJobStageInvalid stepJobStage = iota
	stepJobStageExecution
	stepJobStageSnapshot
	stepJobStageRestore
)

type dispatchJobResult struct {
	effectID   EffectID
	settlement Settlement
	dropped    uint64
}

func newTreeRuntime(
	engine *Engine,
	rootID ProcessID,
	ctx context.Context,
	processes ...*processState,
) *treeRuntime {
	runtime := &treeRuntime{
		engine:      engine,
		rootID:      rootID,
		context:     context.WithoutCancel(requireContext(ctx)),
		commands:    make(chan treeCommand, treeCommandBufferCapacity),
		completions: make(chan treeJobCompletion),
		processes:   make(map[ProcessID]*processState, len(processes)),
		childWaits:  make(map[WaitID]*childWaitRegistration),
		queued:      make(map[ProcessID]struct{}, len(processes)),
		jobs:        make(map[ProcessID]*processJob, len(processes)),
		done:        make(chan struct{}),
	}
	for _, process := range processes {
		runtime.addProcess(process)
	}
	return runtime
}

func (t *treeRuntime) addProcess(process *processState) {
	if t == nil || process == nil || process.controller == nil ||
		process.controller.relation.RootID() != t.rootID {
		panic("agent: invalid tree Process")
	}
	processID := process.controller.processID
	if t.processes[processID] != nil {
		panic("agent: duplicate tree Process")
	}
	process.runtime = t
	process.controller.runtime = t
	t.processes[processID] = process
	if !process.status.Terminal() {
		t.markRunnable(processID)
	}
}

func (t *treeRuntime) run(rootContext context.Context) {
	defer close(t.done)
	for _, process := range t.processes {
		if process.status.Terminal() {
			continue
		}
		if process.restored {
			process.publishEvent(t.context, EventProcessRestored, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
		} else {
			process.publishEvent(t.context, EventProcessStarted, EventPhaseCommitted, 0, EffectID{}, emptyEventPayload())
		}
	}
	stopHostWatch := context.AfterFunc(rootContext, func() {
		select {
		case t.commands <- newTreeProcessCommand(
			t.rootID,
			processCommand{kind: commandHostTerminated, hostErr: rootContext.Err()},
		):
		case <-t.done:
		}
	})
	defer stopHostWatch()

	for {
		if t.tryFreezeCancellation() || t.tryCommand() || t.tryCompletion() || t.advanceOne() {
			continue
		}
		if t.finished() {
			return
		}
		if t.freeze != nil && t.freeze.ready {
			select {
			case command := <-t.commands:
				t.applyCommand(command)
			case <-t.freeze.acquisition.canceled:
				t.releaseFreeze(t.freeze.freeze)
			}
			continue
		}
		if freezeCanceled := t.freezeCanceled(); freezeCanceled != nil {
			select {
			case command := <-t.commands:
				t.applyCommand(command)
			case completion := <-t.completions:
				t.applyCompletion(completion)
			case <-freezeCanceled:
				t.releaseFreeze(t.freeze.freeze)
			}
			continue
		}
		select {
		case command := <-t.commands:
			t.applyCommand(command)
		case completion := <-t.completions:
			t.applyCompletion(completion)
		}
	}
}

func (t *treeRuntime) freezeCanceled() <-chan struct{} {
	if t.freeze == nil || t.freeze.acquisition == nil {
		return nil
	}
	return t.freeze.acquisition.canceled
}

func (t *treeRuntime) tryFreezeCancellation() bool {
	canceled := t.freezeCanceled()
	if canceled == nil {
		return false
	}
	select {
	case <-canceled:
		t.releaseFreeze(t.freeze.freeze)
		return true
	default:
		return false
	}
}

func (t *treeRuntime) tryCommand() bool {
	select {
	case command := <-t.commands:
		t.applyCommand(command)
		return true
	default:
		return false
	}
}

func (t *treeRuntime) tryCompletion() bool {
	if t.freeze != nil && t.freeze.ready {
		return false
	}
	select {
	case completion := <-t.completions:
		t.applyCompletion(completion)
		return true
	default:
		return false
	}
}

func (t *treeRuntime) markRunnable(processID ProcessID) {
	process := t.processes[processID]
	if process == nil || process.status.Terminal() || t.jobs[processID] != nil {
		return
	}
	if _, exists := t.queued[processID]; exists {
		return
	}
	t.queued[processID] = struct{}{}
	t.runnable = append(t.runnable, processID)
}

func (t *treeRuntime) popRunnable() *processState {
	for len(t.runnable) > 0 {
		processID := t.runnable[0]
		t.runnable = t.runnable[1:]
		delete(t.queued, processID)
		process := t.processes[processID]
		if process != nil && !process.status.Terminal() && t.jobs[processID] == nil {
			return process
		}
	}
	return nil
}

func (t *treeRuntime) advanceOne() bool {
	if t.freeze != nil {
		return false
	}
	process := t.popRunnable()
	if process == nil {
		return false
	}
	if process.prepared != nil {
		t.advancePrepared(process)
		return true
	}
	if process.applyPendingControl(t.context) {
		t.finishIfTerminal(process)
		return true
	}
	if process.status == StatusRunning {
		t.startStep(process)
	}
	return true
}

func (t *treeRuntime) advancePrepared(process *processState) {
	if process.pendingControl.hasTerminalIntent() && process.prepared.hasUnknownSettlement() {
		process.discardPrepared()
		process.commitTermination(stepOutcome{})
		t.finishIfTerminal(process)
		return
	}
	if !process.prepared.acknowledged {
		t.startPreparedAcknowledgement(process)
		return
	}
	if process.prepared.hasUnknownSettlement() {
		return
	}
	if !process.prepared.allEffectsSettled() {
		t.startNextEffect(process)
		return
	}
	if err := process.finalizePrepared(t.context); err != nil {
		process.discardPrepared()
		process.fail(FailureKindContract, "engine.finalize.invalid", err)
	}
	t.finishIfTerminal(process)
	if !process.status.Terminal() {
		t.markRunnable(process.controller.processID)
	}
}

func (t *treeRuntime) nextAttempt(process *processState) (processAttempt, bool) {
	if process.attemptSequence == math.MaxUint64 {
		process.fail(
			FailureKindContract,
			"engine.process.attempt_exhausted",
			errors.New("process attempt sequence is exhausted"),
		)
		t.finishIfTerminal(process)
		return 0, false
	}
	process.attemptSequence++
	return processAttempt(process.attemptSequence), true
}

func (t *treeRuntime) startStep(process *processState) {
	if failure := process.stepSchedulingFailure(); failure != nil {
		process.fail(failure.kind, failure.code, failure.cause)
		t.finishIfTerminal(process)
		return
	}
	attempt, ok := t.nextAttempt(process)
	if !ok {
		return
	}
	sequence := process.committedSteps + 1
	process.publishEvent(t.context, EventStepStarted, EventPhaseAttempt, sequence, EffectID{}, emptyEventPayload())
	execution := process.execution
	process.execution = nil
	signals := process.mailbox.pending()
	stepCtx, cancel := context.WithCancel(context.Background())
	t.jobs[process.controller.processID] = &processJob{
		kind: processJobStep, attempt: attempt, cancel: cancel, startedAt: time.Now(),
	}
	go func() {
		transition, err := stepExecution(stepCtx, execution, signals)
		result := stepJobResult{transition: transition, stage: stepJobStageExecution, err: err}
		if err == nil {
			result.stage = stepJobStageSnapshot
			result.candidateState, result.err = captureExecution(execution)
		}
		if result.err == nil {
			result.stage = stepJobStageRestore
			result.candidate, result.err = restoreExecution(
				process.deployment.Definition(), result.candidateState,
			)
		}
		if result.err == nil {
			result.stage = stepJobStageInvalid
		}
		t.completions <- treeJobCompletion{
			processID: process.controller.processID,
			attempt:   attempt,
			kind:      processJobStep,
			step:      result,
		}
	}()
}

func (t *treeRuntime) startPreparedAcknowledgement(process *processState) {
	if process.engine.acknowledger == nil {
		process.prepared.acknowledged = true
		t.markRunnable(process.controller.processID)
		return
	}
	attempt, ok := t.nextAttempt(process)
	if !ok {
		return
	}
	snapshot, err := process.capture()
	if err != nil {
		process.discardPrepared()
		process.fail(FailureKindExternal, "engine.prepared_acknowledgment.failed", err)
		t.finishIfTerminal(process)
		return
	}
	t.jobs[process.controller.processID] = &processJob{
		kind: processJobPreparedAcknowledgement, attempt: attempt,
	}
	go func() {
		err := acknowledgePreparedStep(t.context, process.engine.acknowledger, snapshot)
		t.completions <- treeJobCompletion{
			processID:       process.controller.processID,
			attempt:         attempt,
			kind:            processJobPreparedAcknowledgement,
			acknowledgement: err,
		}
	}()
}

func (t *treeRuntime) startNextEffect(process *processState) {
	for index := range process.prepared.wire.Effects {
		record := &process.prepared.wire.Effects[index]
		if record.Settlement != nil {
			continue
		}
		if record.Effect.Target() == EffectTargetFramework {
			startedAt := process.publishEffectStarted(
				t.context, process.prepared.wire.StepSequence, record.ID, EffectTargetFramework,
			)
			operation, err := decodeFrameworkEffectOperation(record.Effect.Payload())
			if err == nil && operation == frameworkEffectStartChild {
				t.startChild(process, record, startedAt)
				return
			}
			process.dispatchFrameworkEffect(t.context, record)
			process.publishSettlementEvent(
				t.context, record.ID, EffectTargetFramework, record.Settlement.Status(), startedAt,
			)
			t.markRunnable(process.controller.processID)
			return
		}
		t.startDispatch(process, uint32(index), *record)
		return
	}
}

func (t *treeRuntime) startChild(
	process *processState,
	record *preparedEffectWire,
	startedAt time.Time,
) {
	spec, err := decodeChildStartEffect(record.Effect.Payload())
	if err != nil {
		process.markFrameworkEffectUnknown(record)
		process.publishSettlementEvent(
			t.context, record.ID, EffectTargetFramework, record.Settlement.Status(), startedAt,
		)
		t.markRunnable(process.controller.processID)
		return
	}
	attempt, ok := t.nextAttempt(process)
	if !ok {
		return
	}
	preparation := process.prepareChildStart(record.ID, spec)
	if preparation.plan == nil {
		t.settleChildStart(process, record.ID, preparation.result, startedAt)
		t.markRunnable(process.controller.processID)
		return
	}
	job := &processJob{
		kind: processJobChildStart, attempt: attempt, effectID: record.ID,
		childStart: preparation.plan, startedAt: startedAt,
	}
	t.jobs[process.controller.processID] = job
	go func() {
		result := preparation.plan.execute(t.context)
		t.completions <- treeJobCompletion{
			processID:  process.controller.processID,
			attempt:    attempt,
			kind:       processJobChildStart,
			childStart: result,
		}
	}()
}

func (t *treeRuntime) startDispatch(
	process *processState,
	batchIndex uint32,
	record preparedEffectWire,
) {
	attempt, ok := t.nextAttempt(process)
	if !ok {
		return
	}
	request := newEffectRequest(
		process.controller.processID,
		process.controller.deploymentRef,
		process.controller.relation,
		process.prepared.wire.StepSequence,
		batchIndex,
		record.ID,
		record.Effect,
	)
	startedAt := process.publishEffectStarted(
		t.context, process.prepared.wire.StepSequence, record.ID, EffectTargetDispatcher,
	)
	job := &processJob{
		kind:      processJobDispatch,
		attempt:   attempt,
		effectID:  record.ID,
		startedAt: startedAt,
	}
	t.jobs[process.controller.processID] = job
	var deltaSequence atomic.Uint64
	var dropped atomic.Uint64
	var acceptingDeltas atomic.Bool
	acceptingDeltas.Store(true)
	emit := func(payload json.RawMessage) {
		if !acceptingDeltas.Load() {
			return
		}
		sequence := deltaSequence.Add(1)
		delta, err := newDelta(process.controller.processID, record.ID, sequence, time.Now(), payload)
		if err != nil || !process.engine.observation.offerDelta(t.context, delta) {
			dropped.Add(1)
		}
	}
	go func() {
		settlement, err := dispatchEffect(
			t.context,
			process.deployment.effectDispatcher(),
			request,
			emit,
		)
		acceptingDeltas.Store(false)
		if err != nil || !settlement.Valid() || settlement.EffectID() != record.ID {
			settlement, _ = NewSettlement(record.ID, SettlementStatusUnknown, json.RawMessage(nullJSON))
		}
		t.completions <- treeJobCompletion{
			processID: process.controller.processID,
			attempt:   attempt,
			kind:      processJobDispatch,
			dispatch: dispatchJobResult{
				effectID:   record.ID,
				settlement: settlement,
				dropped:    dropped.Load(),
			},
		}
	}()
}

func (t *treeRuntime) applyCompletion(completion treeJobCompletion) {
	process := t.processes[completion.processID]
	job := t.jobs[completion.processID]
	if process == nil || job == nil || job.kind != completion.kind || job.attempt != completion.attempt {
		return
	}
	delete(t.jobs, completion.processID)
	if job.cancel != nil {
		job.cancel()
	}
	if job.stale {
		if completion.kind == processJobStep {
			process.discardExecution()
		}
		if t.freeze == nil {
			t.markRunnable(completion.processID)
		}
		t.completeFreeze()
		return
	}
	switch completion.kind {
	case processJobStep:
		t.applyStepCompletion(process, job, completion.step)
	case processJobPreparedAcknowledgement:
		if completion.acknowledgement != nil {
			process.discardPrepared()
			process.fail(
				FailureKindExternal,
				"engine.prepared_acknowledgment.failed",
				completion.acknowledgement,
			)
		} else {
			process.prepared.acknowledged = true
		}
	case processJobDispatch:
		t.applyDispatchCompletion(process, job, completion.dispatch)
	case processJobChildStart:
		t.applyChildStartCompletion(process, job, completion.childStart)
	}
	t.finishIfTerminal(process)
	if !process.status.Terminal() {
		t.markRunnable(completion.processID)
	}
	t.completeFreeze()
}

func (t *treeRuntime) applyChildStartCompletion(
	parent *processState,
	job *processJob,
	result childStartJobResult,
) {
	plan := job.childStart
	if plan == nil {
		parent.markFrameworkEffectUnknownByID(job.effectID)
		return
	}
	if result.started() {
		controller := newProcessController(
			plan.relation,
			result.deployment.DeploymentRef(),
			plan.spec.Budget,
			plan.spec.Capabilities,
			plan.treeLimits,
			result.startedAt,
			StatusRunning,
		)
		child := newProcessState(
			plan.engine, controller, result.deployment, result.execution,
			result.state, result.startedAt, plan.limits,
		)
		t.addProcess(child)
		plan.engine.publishReservedProcess(controller)
	} else {
		plan.engine.discardProcessStartReservation(plan.childID)
		parent.releaseChildBudget(plan.spec.Budget)
	}
	t.settleChildStart(parent, job.effectID, result.result, job.startedAt)
}

func (t *treeRuntime) settleChildStart(
	parent *processState,
	effectID EffectID,
	result ChildStartResult,
	startedAt time.Time,
) {
	if parent.prepared == nil {
		return
	}
	for index := range parent.prepared.wire.Effects {
		record := &parent.prepared.wire.Effects[index]
		if record.ID != effectID || record.Settlement != nil {
			continue
		}
		payload, err := encodeChildStartResult(result)
		if err != nil {
			parent.markFrameworkEffectUnknown(record)
		} else {
			status := SettlementStatusSucceeded
			if _, failed := result.Failure(); failed {
				status = SettlementStatusFailed
			}
			settlement, settlementErr := NewSettlement(effectID, status, payload)
			if settlementErr != nil {
				parent.markFrameworkEffectUnknown(record)
			} else {
				record.Settlement = &settlement
			}
		}
		parent.publishSettlementEvent(
			t.context, effectID, EffectTargetFramework, record.Settlement.Status(), startedAt,
		)
		return
	}
}

func (t *treeRuntime) applyStepCompletion(
	process *processState,
	job *processJob,
	result stepJobResult,
) {
	stepStatus := StepStatusSucceeded
	if result.err != nil {
		stepStatus = StepStatusFailed
	}
	payload, _ := json.Marshal(stepFinishedEventPayload{
		StepStatus: stepStatus,
		DurationMS: time.Since(job.startedAt).Milliseconds(),
	})
	sequence := process.committedSteps + 1
	process.publishEvent(t.context, EventStepFinished, EventPhaseAttempt, sequence, EffectID{}, payload)
	if result.err != nil {
		process.discardExecution()
		code := "execution.step.failed"
		switch result.stage {
		case stepJobStageSnapshot:
			code = "execution.snapshot.failed"
		case stepJobStageRestore:
			code = "execution.snapshot.unrestorable"
		}
		process.fail(failureKindForError(result.err), code, result.err)
		return
	}
	if failure := process.prepareStepResult(t.context, result); failure != nil {
		process.discardExecution()
		process.fail(failure.kind, failure.code, failure.cause)
	}
}

func (t *treeRuntime) applyDispatchCompletion(
	process *processState,
	job *processJob,
	result dispatchJobResult,
) {
	if process.prepared == nil {
		return
	}
	for index := range process.prepared.wire.Effects {
		record := &process.prepared.wire.Effects[index]
		if record.ID != result.effectID || record.Settlement != nil {
			continue
		}
		settlement := result.settlement
		record.Settlement = &settlement
		if result.dropped > 0 {
			process.usage.DroppedDeltas = saturatingCountAdd(
				process.usage.DroppedDeltas,
				result.dropped,
			)
			process.updateView()
			payload, _ := json.Marshal(deltaDroppedEventPayload{DroppedDeltaCount: result.dropped})
			process.publishEvent(
				t.context,
				EventDeltaDropped,
				EventPhaseAttempt,
				process.prepared.wire.StepSequence,
				record.ID,
				payload,
			)
		}
		process.publishSettlementEvent(
			t.context,
			record.ID,
			EffectTargetDispatcher,
			settlement.Status(),
			job.startedAt,
		)
		return
	}
}

func (t *treeRuntime) applyCommand(command treeCommand) {
	switch command.kind {
	case treeCommandAcquireFreeze:
		t.acquireFreeze(command.acquisition)
		return
	case treeCommandReleaseFreeze:
		err := t.releaseFreeze(command.freeze)
		command.response <- err
		return
	case treeCommandApplyFreeze:
		err := t.applyFreeze(command.freeze, command.projection)
		command.response <- err
		return
	case treeCommandProcess:
		if t.freeze != nil {
			t.freeze.deferred = append(t.freeze.deferred, command)
			return
		}
	default:
		if command.response != nil {
			command.response <- ErrEngineQuiescenceUnavailable
		}
		return
	}
	process := t.processes[command.processID]
	if process == nil {
		command.process.reply(processResponse{err: ErrProcessNotRunning})
		return
	}
	processCommand := command.process
	if processCommand.kind == commandHostTerminated {
		if !process.status.Terminal() {
			process.recordHostTermination(processCommand.hostErr)
			t.invalidateStep(process)
			t.markRunnable(process.controller.processID)
		}
		return
	}
	process.applyCommand(t.context, processCommand)
	if process.pendingControl.hasTerminalIntent() || process.pendingControl.pauseReason != "" {
		t.invalidateStep(process)
	}
	t.finishIfTerminal(process)
	if !process.status.Terminal() {
		t.markRunnable(process.controller.processID)
	}
}

func (t *treeRuntime) acquireFreeze(acquisition *treeFreezeAcquisition) {
	if acquisition == nil || acquisition.response == nil || acquisition.canceled == nil ||
		(acquisition.mode != treeFreezeModeSnapshot && acquisition.mode != treeFreezeModeExclusive) ||
		t.freeze != nil {
		if acquisition != nil && acquisition.response != nil {
			acquisition.response <- treeFreezeAcquisitionResult{err: ErrEngineQuiescenceUnavailable}
		}
		return
	}
	freeze := &treeFreeze{runtime: t}
	t.freeze = &activeTreeFreeze{acquisition: acquisition, freeze: freeze}
	if acquisition.mode == treeFreezeModeExclusive {
		for _, process := range t.processes {
			t.invalidateStep(process)
		}
	}
	t.completeFreeze()
}

func (t *treeRuntime) completeFreeze() {
	if t.freeze == nil || t.freeze.ready || t.freezeBlockedByJob() {
		return
	}
	snapshot, err := t.captureTree()
	if err != nil {
		acquisition := t.freeze.acquisition
		t.releaseFreeze(t.freeze.freeze)
		acquisition.response <- treeFreezeAcquisitionResult{err: err}
		return
	}
	t.freeze.ready = true
	t.freeze.acquisition.response <- treeFreezeAcquisitionResult{
		freeze: t.freeze.freeze, snapshot: snapshot,
	}
}

func (t *treeRuntime) freezeBlockedByJob() bool {
	if t.freeze == nil {
		return false
	}
	if t.freeze.acquisition.mode == treeFreezeModeExclusive {
		return len(t.jobs) != 0
	}
	for _, job := range t.jobs {
		if job.kind != processJobStep {
			return true
		}
	}
	return false
}

func (t *treeRuntime) captureTree() (TreeSnapshot, error) {
	wire := treeSnapshotWire{SchemaVersion: treeSnapshotSchemaVersion, RootID: t.rootID}
	for _, process := range t.processes {
		snapshot, err := process.capture()
		if err != nil {
			return TreeSnapshot{}, err
		}
		wire.ProcessSnapshots = append(wire.ProcessSnapshots, snapshot)
	}
	for _, registration := range t.childWaits {
		wire.ChildWaits = append(wire.ChildWaits, childWaitSnapshotWire{
			ParentProcessID: registration.parent,
			WaitID:          registration.waitID,
			Spec:            childWaitSpecWireFromValue(registration.spec),
		})
	}
	return newTreeSnapshot(wire)
}

func (t *treeRuntime) releaseFreeze(freeze *treeFreeze) error {
	if t.freeze == nil || freeze == nil || t.freeze.freeze != freeze {
		return ErrEngineQuiescenceUnavailable
	}
	deferred := t.freeze.deferred
	t.freeze = nil
	for _, process := range t.processes {
		if !process.status.Terminal() {
			t.markRunnable(process.controller.processID)
		}
	}
	for _, command := range deferred {
		t.applyCommand(command)
	}
	return nil
}

func (t *treeRuntime) applyFreeze(
	freeze *treeFreeze,
	projection *treeStateProjection,
) error {
	if t.freeze == nil || !t.freeze.ready || freeze == nil || t.freeze.freeze != freeze ||
		projection == nil {
		return ErrInvalidPreparedWaitingSubtreeCancellation
	}
	for _, change := range projection.changes {
		process := t.processes[change.processID]
		if err := change.validateSource(process); err != nil {
			return err
		}
	}
	for _, change := range projection.changes {
		process := t.processes[change.processID]
		change.apply(t.context, process)
	}
	t.childWaits = make(map[WaitID]*childWaitRegistration, len(projection.childWaits))
	for _, registration := range projection.childWaits {
		t.childWaits[registration.waitID] = registration
	}
	for _, change := range projection.changes {
		t.finishIfTerminal(t.processes[change.processID])
	}
	return t.releaseFreeze(freeze)
}

func (t *treeRuntime) invalidateStep(process *processState) {
	job := t.jobs[process.controller.processID]
	if job == nil || job.kind != processJobStep || job.stale {
		return
	}
	job.stale = true
	job.cancel()
}

func (t *treeRuntime) finishIfTerminal(process *processState) {
	if process == nil || !process.status.Terminal() {
		return
	}
	select {
	case <-process.controller.done:
		return
	default:
	}
	eventPayload := processFinishedEventPayload{
		ProcessStatus:    process.status,
		TerminationCause: process.termination.Cause(),
	}
	if failure, failed := process.termination.Failure(); failed {
		eventPayload.FailureKind = failure.Kind()
		eventPayload.FailureCode = failure.Code()
	}
	payload, _ := json.Marshal(eventPayload)
	process.publishEvent(t.context, EventProcessFinished, EventPhaseCommitted, 0, EffectID{}, payload)
	snapshot, err := process.capture()
	process.controller.complete(process.result(), snapshot, err)
	t.processFinished(process)
	process.controller.markTreeSettled()
}

func (t *treeRuntime) processFinished(process *processState) {
	processID := process.controller.processID
	for waitID, registration := range t.childWaits {
		if registration.parent == processID {
			delete(t.childWaits, waitID)
		}
	}
	for _, child := range t.processes {
		parentID, isChild := child.controller.relation.ParentID()
		if isChild && parentID == processID && !child.status.Terminal() {
			child.recordParentTermination(process.termination)
			t.invalidateStep(child)
			t.markRunnable(child.controller.processID)
		}
	}
	for _, registration := range orderedChildWaitRegistrations(t.childWaits) {
		if registration.delivered || !containsProcessID(registration.spec.Children, processID) {
			continue
		}
		outcomes, satisfied := t.childWaitOutcomes(registration)
		if !satisfied {
			continue
		}
		signal, err := encodeChildrenCompleted(registration.waitID, registration.spec.Key, outcomes)
		if err != nil {
			continue
		}
		parent := t.processes[registration.parent]
		if parent != nil && parent.deliverChildrenCompleted(t.context, signal) {
			registration.delivered = true
			t.markRunnable(parent.controller.processID)
		}
	}
}

func orderedChildWaitRegistrations(
	registrations map[WaitID]*childWaitRegistration,
) []*childWaitRegistration {
	ordered := make([]*childWaitRegistration, 0, len(registrations))
	for _, registration := range registrations {
		ordered = append(ordered, registration)
	}
	slices.SortFunc(ordered, func(left, right *childWaitRegistration) int {
		return cmp.Compare(left.waitID.String(), right.waitID.String())
	})
	return ordered
}

func containsProcessID(processes []ProcessID, processID ProcessID) bool {
	for _, candidate := range processes {
		if candidate == processID {
			return true
		}
	}
	return false
}

func (t *treeRuntime) childWaitOutcomes(
	registration *childWaitRegistration,
) ([]ChildOutcome, bool) {
	outcomes := make([]ChildOutcome, 0, len(registration.spec.Children))
	for _, childID := range registration.spec.Children {
		child := t.processes[childID]
		if child == nil {
			return nil, false
		}
		select {
		case <-child.controller.done:
			key, _ := child.controller.relation.ChildKey()
			outcomes = append(outcomes, ChildOutcome{key: key, result: child.controller.terminalResult()})
		default:
		}
	}
	required, err := registration.spec.Condition.required(len(registration.spec.Children))
	return outcomes, err == nil && uint32(len(outcomes)) >= required
}

func (t *treeRuntime) registerChildWait(
	parentID ProcessID,
	waitID WaitID,
	spec ChildWaitSpec,
) (Signal, bool, error) {
	if !parentID.Valid() || !waitID.Valid() || !spec.Valid() || t.processes[parentID] == nil {
		return Signal{}, false, ErrInvalidChildWait
	}
	if t.childWaits[waitID] != nil {
		return Signal{}, false, ErrInvalidChildWait
	}
	for _, childID := range spec.Children {
		child := t.processes[childID]
		if child == nil {
			return Signal{}, false, ErrInvalidChildWait
		}
		actualParent, isChild := child.controller.relation.ParentID()
		if !isChild || actualParent != parentID {
			return Signal{}, false, ErrInvalidChildWait
		}
	}
	registration := &childWaitRegistration{
		parent: parentID,
		waitID: waitID,
		spec:   cloneChildWaitSpec(spec),
	}
	t.childWaits[waitID] = registration
	outcomes, satisfied := t.childWaitOutcomes(registration)
	if !satisfied {
		return Signal{}, false, nil
	}
	signal, err := encodeChildrenCompleted(waitID, spec.Key, outcomes)
	if err != nil {
		delete(t.childWaits, waitID)
		return Signal{}, false, err
	}
	registration.delivered = true
	return signal, true, nil
}

func (t *treeRuntime) unregisterChildWait(waitID WaitID) {
	delete(t.childWaits, waitID)
}

func (t *treeRuntime) finished() bool {
	if t.freeze != nil || len(t.jobs) != 0 {
		return false
	}
	for _, process := range t.processes {
		if !process.status.Terminal() {
			return false
		}
	}
	return true
}
