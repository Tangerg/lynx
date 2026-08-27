package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

var (
	// ErrProcessFinished reports a control request made after terminal commit.
	ErrProcessFinished = errors.New("agent: process has finished")
	// ErrProcessNotRunning reports an operation requiring an active Process.
	ErrProcessNotRunning = errors.New("agent: process is not running")
	// ErrEffectNotPending reports resolution of an Effect without an unknown
	// pending settlement.
	ErrEffectNotPending = errors.New("agent: effect does not require resolution")
	// ErrInvalidProcessControl reports a malformed Process lifecycle request.
	ErrInvalidProcessControl = errors.New("agent: invalid process control request")
)

// Process is an Engine-issued handle to one managed execution. Its fields and
// construction remain private so a caller cannot create a second lifecycle
// owner. Methods only submit control-plane requests to the owning Engine loop.
// Except for RequestCancellation, ctx bounds both command submission and
// response waiting. Once the Engine loop receives a command, canceling ctx does
// not revoke it.
type Process struct {
	controller *processController
}

// ID returns the stable Process identity.
func (p *Process) ID() ProcessID {
	if p == nil || p.controller == nil {
		return ProcessID{}
	}
	return p.controller.processID
}

// DeploymentRef returns the exact Definition and dispatcher binding identity.
func (p *Process) DeploymentRef() DeploymentRef {
	if p == nil || p.controller == nil {
		return DeploymentRef{}
	}
	return p.controller.deploymentRef
}

// Relation returns the immutable parent/root/depth location assigned by the
// Engine. It is a root relation for Processes created through Engine.Start.
func (p *Process) Relation() ProcessRelation {
	if p == nil || p.controller == nil {
		return ProcessRelation{}
	}
	return p.controller.relation
}

// StartedAt returns when the Engine created this Process.
func (p *Process) StartedAt() time.Time {
	if p == nil || p.controller == nil {
		return time.Time{}
	}
	return p.controller.startedAt
}

// Status returns the latest committed common lifecycle status.
func (p *Process) Status() Status {
	if p == nil || p.controller == nil {
		return StatusInvalid
	}
	return p.controller.status()
}

// Usage returns the latest Framework-owned counters.
func (p *Process) Usage() Usage {
	if p == nil || p.controller == nil {
		return Usage{}
	}
	return p.controller.usage()
}

// WaitID returns the current externally addressable wait while Status is
// Waiting. The payload schema and meaning remain owned by the Strategy.
func (p *Process) WaitID() (WaitID, bool) {
	if p == nil || p.controller == nil {
		return WaitID{}, false
	}
	return p.controller.waitID()
}

// DeliverSignal submits immutable Strategy input. Running input is consumed only at
// the next Strategy-safe Step boundary; Waiting input must address WaitID.
// accepted is false, with nil error, when SignalID was already accepted.
func (p *Process) DeliverSignal(ctx context.Context, request SignalRequest) (accepted bool, err error) {
	response, err := p.request(ctx, processCommand{kind: commandDeliver, signalRequest: request})
	return response.accepted, err
}

// DeliverSignals atomically appends an ordered Signal batch. This is useful
// when one WaitID-addressed response and ordinary follow-up input must become
// visible at the same safe Strategy boundary. Either the complete batch is
// accepted in order or the mailbox remains unchanged.
func (p *Process) DeliverSignals(ctx context.Context, requests ...SignalRequest) (accepted bool, err error) {
	if len(requests) == 0 {
		return false, ErrInvalidSignalRequest
	}
	owned := slices.Clone(requests)
	response, err := p.request(ctx, processCommand{kind: commandDeliverBatch, signalRequests: owned})
	return response.accepted, err
}

// Pause requests a scheduling pause at the next safe Step boundary. An
// in-flight Effect is allowed to settle before the pause becomes visible.
func (p *Process) Pause(ctx context.Context, reason string) error {
	_, err := p.request(ctx, processCommand{kind: commandPause, reason: reason})
	return err
}

// Resume makes an explicitly Paused Process schedulable again. Waiting is
// resumed only by a Signal addressed to its current WaitID.
func (p *Process) Resume(ctx context.Context) error {
	_, err := p.request(ctx, processCommand{kind: commandResume})
	return err
}

// RequestCancellation submits a caller-owned cancellation intent. A nil error
// means the request entered the owning Engine loop's queue; it does not mean the
// Process has reached a safe boundary or become terminal. Once submitted, ctx
// cancellation cannot revoke the request. The first committed cancellation
// intent maps to StatusCanceled with a host-cancellation cause.
func (p *Process) RequestCancellation(ctx context.Context, reason string) error {
	if p == nil || p.controller == nil {
		return ErrProcessNotRunning
	}
	intent, err := newCancellationIntent(cancellationOwnerHost, reason)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProcessControl, err)
	}
	ctx = contextOrBackground(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case p.controller.commands <- processCommand{kind: commandCancel, cancellationIntent: intent}:
		return nil
	case <-p.controller.done:
		return ErrProcessFinished
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Kill records the Engine control plane's highest-priority terminal intent.
// It does not silently abandon an in-flight Effect; settlement finishes first.
func (p *Process) Kill(ctx context.Context, reason string) error {
	_, err := p.request(ctx, processCommand{kind: commandKill, reason: reason})
	return err
}

// ResolveEffect supplies a definite result after an Effect attempt became
// unknown. The Engine never converts unknown into retry or success implicitly.
func (p *Process) ResolveEffect(ctx context.Context, settlement Settlement) error {
	_, err := p.request(ctx, processCommand{kind: commandResolveEffect, settlement: settlement})
	return err
}

// UnknownEffectIDs returns stable identities whose external outcome requires an
// explicit ResolveEffect decision. Payloads remain owned by the Dispatcher.
func (p *Process) UnknownEffectIDs(ctx context.Context) ([]EffectID, error) {
	response, err := p.request(ctx, processCommand{kind: commandQueryUnknownEffectIDs})
	return response.unknownEffectIDs, err
}

// Capture returns a consistent last-stable or prepared-step snapshot. Capture
// does not imply that the caller persisted it durably.
func (p *Process) Capture(ctx context.Context) (Snapshot, error) {
	if p == nil || p.controller == nil {
		return Snapshot{}, ErrProcessNotRunning
	}
	if snapshot, ok, err := p.controller.finishedSnapshot(); ok {
		return snapshot, err
	}
	response, err := p.request(ctx, processCommand{kind: commandCapture})
	return response.snapshot, err
}

// Await waits for the immutable terminal result and the Engine's immediate
// parent/child bookkeeping for that termination. Canceling ctx stops only the
// wait; Process cancellation is explicit or follows the context passed to Start.
func (p *Process) Await(ctx context.Context) (Result, error) {
	if p == nil || p.controller == nil {
		return Result{}, ErrProcessNotRunning
	}
	ctx = contextOrBackground(ctx)
	select {
	case <-p.controller.treeSettled:
		return p.controller.terminalResult(), nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (p *Process) request(ctx context.Context, command processCommand) (processResponse, error) {
	if p == nil || p.controller == nil {
		return processResponse{}, ErrProcessNotRunning
	}
	ctx = contextOrBackground(ctx)
	command.response = make(chan processResponse, 1)
	select {
	case p.controller.commands <- command:
	case <-p.controller.done:
		return processResponse{}, ErrProcessFinished
	case <-ctx.Done():
		return processResponse{}, ctx.Err()
	}
	select {
	case response := <-command.response:
		return response, response.err
	case <-p.controller.done:
		select {
		case response := <-command.response:
			return response, response.err
		default:
			return processResponse{}, ErrProcessFinished
		}
	case <-ctx.Done():
		return processResponse{}, ctx.Err()
	}
}

// Result is the immutable terminal outcome of one Process. A failed or
// canceled execution is represented by Termination, not by Await's error.
type Result struct {
	processID   ProcessID
	startedAt   time.Time
	finishedAt  time.Time
	output      Output
	termination Termination
	usage       Usage
}

// ProcessID returns the completed Process identity.
func (r Result) ProcessID() ProcessID { return r.processID }

// StartedAt returns the lifecycle start time.
func (r Result) StartedAt() time.Time { return r.startedAt }

// FinishedAt returns the committed terminal time.
func (r Result) FinishedAt() time.Time { return r.finishedAt }

// Status returns the terminal lifecycle state.
func (r Result) Status() Status { return r.termination.Status() }

// Termination returns the stable terminal cause and optional Failure.
func (r Result) Termination() Termination { return r.termination }

// Usage returns the final Framework-owned resource counters.
func (r Result) Usage() Usage { return r.usage }

// Output returns the final semantic result only for StatusCompleted.
func (r Result) Output() (Output, bool) { return r.output, r.output.Valid() }

// Valid reports whether the result contains one complete terminal outcome.
func (r Result) Valid() bool {
	if !r.processID.Valid() || r.startedAt.IsZero() || r.finishedAt.Before(r.startedAt) || !r.termination.Valid() {
		return false
	}
	return r.termination.Status() == StatusCompleted && r.output.Valid() ||
		r.termination.Status() != StatusCompleted && !r.output.Valid()
}

type processController struct {
	processID          ProcessID
	deploymentRef      DeploymentRef
	relation           ProcessRelation
	childRequestDigest Digest
	budget             Budget
	capabilities       CapabilitySet
	treeLimits         TreeLimits
	startedAt          time.Time
	commands           chan processCommand
	done               chan struct{}
	treeSettled        chan struct{}

	viewMu              sync.RWMutex
	viewStatus          Status
	viewWaitID          WaitID
	viewUsage           Usage
	result              Result
	terminalSnapshot    Snapshot
	terminalSnapshotErr error
}

func newProcessController(
	relation ProcessRelation,
	deploymentRef DeploymentRef,
	budget Budget,
	capabilities CapabilitySet,
	treeLimits TreeLimits,
	startedAt time.Time,
	status Status,
) *processController {
	return &processController{
		processID: relation.ProcessID(), deploymentRef: deploymentRef, relation: relation,
		budget: budget, capabilities: capabilities, treeLimits: treeLimits, startedAt: startedAt,
		commands: make(chan processCommand, 32), done: make(chan struct{}),
		treeSettled: make(chan struct{}), viewStatus: status,
	}
}

// Budget returns the fixed non-renewable allocation assigned to this Process.
func (p *Process) Budget() Budget {
	if p == nil || p.controller == nil {
		return Budget{}
	}
	return p.controller.budget
}

// Capabilities returns the immutable authority set assigned to this Process.
func (p *Process) Capabilities() CapabilitySet {
	if p == nil || p.controller == nil {
		return CapabilitySet{}
	}
	return p.controller.capabilities
}

func (p *processController) status() Status {
	p.viewMu.RLock()
	defer p.viewMu.RUnlock()
	return p.viewStatus
}

func (p *processController) waitID() (WaitID, bool) {
	p.viewMu.RLock()
	defer p.viewMu.RUnlock()
	return p.viewWaitID, p.viewStatus == StatusWaiting && p.viewWaitID.Valid()
}

func (p *processController) usage() Usage {
	p.viewMu.RLock()
	defer p.viewMu.RUnlock()
	return p.viewUsage
}

func (p *processController) updateView(status Status, waitID WaitID, usage Usage) {
	p.viewMu.Lock()
	p.viewStatus = status
	p.viewWaitID = waitID
	p.viewUsage = usage
	p.viewMu.Unlock()
}

func (p *processController) complete(result Result, snapshot Snapshot, captureErr error) {
	p.viewMu.Lock()
	p.viewStatus = result.Status()
	p.viewWaitID = WaitID{}
	p.viewUsage = result.usage
	p.result = result
	p.terminalSnapshot = snapshot
	p.terminalSnapshotErr = captureErr
	p.viewMu.Unlock()
	close(p.done)
}

func (p *processController) markTreeSettled() { close(p.treeSettled) }

func (p *processController) terminalResult() Result {
	p.viewMu.RLock()
	defer p.viewMu.RUnlock()
	return p.result
}

func (p *processController) finishedSnapshot() (Snapshot, bool, error) {
	select {
	case <-p.done:
		p.viewMu.RLock()
		defer p.viewMu.RUnlock()
		return p.terminalSnapshot, true, p.terminalSnapshotErr
	default:
		return Snapshot{}, false, nil
	}
}

type commandKind uint8

const (
	commandInvalid commandKind = iota
	commandDeliver
	commandDeliverBatch
	commandPause
	commandResume
	commandCancel
	commandKill
	commandResolveEffect
	commandQueryUnknownEffectIDs
	commandCapture
	commandChildrenCompleted
	commandParentTerminated
	commandQuiesce
	commandStagePreparedProcessState
)

type processCommand struct {
	kind                commandKind
	signalRequest       SignalRequest
	signalRequests      []SignalRequest
	settlement          Settlement
	internalSignal      Signal
	parentTermination   Termination
	cancellationIntent  cancellationIntent
	preparedStateChange *preparedProcessStateChange
	release             <-chan struct{}
	reason              string
	response            chan processResponse
}

type processResponse struct {
	accepted         bool
	snapshot         Snapshot
	unknownEffectIDs []EffectID
	err              error
}

func (p processCommand) reply(response processResponse) {
	if p.response == nil {
		return
	}
	p.response <- response
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
