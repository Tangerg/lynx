package agent2

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrProcessFinished   = errors.New("agent: process has finished")
	ErrProcessNotRunning = errors.New("agent: process is not running")
	ErrEffectNotPending  = errors.New("agent: effect does not require resolution")
	ErrInvalidControl    = errors.New("agent: invalid process control request")
)

// Process is an Engine-issued handle to one managed execution. Its fields and
// construction remain private so a caller cannot create a second lifecycle
// owner. Methods only submit control-plane requests to the owning Engine loop.
type Process struct {
	controller *processController
}

// ID returns the stable Process identity.
func (process *Process) ID() ProcessID {
	if process == nil || process.controller == nil {
		return ProcessID{}
	}
	return process.controller.id
}

// DeploymentRef returns the exact Definition and dispatcher binding identity.
func (process *Process) DeploymentRef() DeploymentRef {
	if process == nil || process.controller == nil {
		return DeploymentRef{}
	}
	return process.controller.deployment
}

// Relation returns the immutable parent/root/depth location assigned by the
// Engine. It is a root relation for Processes created through Engine.Start.
func (process *Process) Relation() ProcessRelation {
	if process == nil || process.controller == nil {
		return ProcessRelation{}
	}
	return process.controller.relation
}

// StartedAt returns when the Engine created this Process.
func (process *Process) StartedAt() time.Time {
	if process == nil || process.controller == nil {
		return time.Time{}
	}
	return process.controller.startedAt
}

// Status returns the latest committed common lifecycle status.
func (process *Process) Status() Status {
	if process == nil || process.controller == nil {
		return StatusInvalid
	}
	return process.controller.status()
}

// Usage returns the latest Framework-owned counters.
func (process *Process) Usage() Usage {
	if process == nil || process.controller == nil {
		return Usage{}
	}
	return process.controller.usage()
}

// WaitID returns the current externally addressable wait while Status is
// Waiting. The payload schema and meaning remain owned by the Strategy.
func (process *Process) WaitID() (WaitID, bool) {
	if process == nil || process.controller == nil {
		return WaitID{}, false
	}
	return process.controller.waitID()
}

// DeliverSignal submits immutable Strategy input. Running input is consumed only at
// the next Strategy-safe Step boundary; Waiting input must address WaitID.
// accepted is false, with nil error, when SignalID was already accepted.
func (process *Process) DeliverSignal(ctx context.Context, request SignalRequest) (accepted bool, err error) {
	response, err := process.request(ctx, processCommand{kind: commandDeliver, signal: request})
	return response.accepted, err
}

// Pause requests a scheduling pause at the next safe Step boundary. An
// in-flight Effect is allowed to settle before the pause becomes visible.
func (process *Process) Pause(ctx context.Context, reason string) error {
	_, err := process.request(ctx, processCommand{kind: commandPause, reason: reason})
	return err
}

// Resume makes an explicitly Paused Process schedulable again. Waiting is
// resumed only by a Signal addressed to its current WaitID.
func (process *Process) Resume(ctx context.Context) error {
	_, err := process.request(ctx, processCommand{kind: commandResume})
	return err
}

// Cancel records a caller-owned cancellation. It is committed at the next safe
// boundary and maps to StatusCancelled with a host-cancellation cause.
func (process *Process) Cancel(ctx context.Context, reason string) error {
	_, err := process.request(ctx, processCommand{kind: commandCancel, reason: reason})
	return err
}

// Kill records the Engine control plane's highest-priority terminal intent.
// It does not silently abandon an in-flight Effect; settlement finishes first.
func (process *Process) Kill(ctx context.Context, reason string) error {
	_, err := process.request(ctx, processCommand{kind: commandKill, reason: reason})
	return err
}

// ResolveEffect supplies a definite result after an Effect attempt became
// unknown. The Engine never converts unknown into retry or success implicitly.
func (process *Process) ResolveEffect(ctx context.Context, settlement Settlement) error {
	_, err := process.request(ctx, processCommand{kind: commandResolveEffect, settlement: settlement})
	return err
}

// UnknownEffectIDs returns stable identities whose external outcome requires an
// explicit ResolveEffect decision. Payloads remain owned by the Dispatcher.
func (process *Process) UnknownEffectIDs(ctx context.Context) ([]EffectID, error) {
	response, err := process.request(ctx, processCommand{kind: commandQueryUnknownEffectIDs})
	return response.effectIDs, err
}

// Capture returns a consistent last-stable or prepared-step snapshot. Capture
// does not imply that the caller persisted it durably.
func (process *Process) Capture(ctx context.Context) (Snapshot, error) {
	if process == nil || process.controller == nil {
		return Snapshot{}, ErrProcessNotRunning
	}
	if snapshot, err, ok := process.controller.finishedSnapshot(); ok {
		return snapshot, err
	}
	response, err := process.request(ctx, processCommand{kind: commandCapture})
	return response.snapshot, err
}

// Await waits for the immutable terminal result. Cancelling ctx stops only the
// wait; Process cancellation is explicit or follows the context passed to Start.
func (process *Process) Await(ctx context.Context) (Result, error) {
	if process == nil || process.controller == nil {
		return Result{}, ErrProcessNotRunning
	}
	ctx = normalizedContext(ctx)
	select {
	case <-process.controller.done:
		return process.controller.terminalResult(), nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (process *Process) request(ctx context.Context, command processCommand) (processResponse, error) {
	if process == nil || process.controller == nil {
		return processResponse{}, ErrProcessNotRunning
	}
	ctx = normalizedContext(ctx)
	command.response = make(chan processResponse, 1)
	select {
	case process.controller.commands <- command:
	case <-process.controller.done:
		return processResponse{}, ErrProcessFinished
	case <-ctx.Done():
		return processResponse{}, ctx.Err()
	}
	select {
	case response := <-command.response:
		return response, response.err
	case <-process.controller.done:
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
// cancelled execution is represented by Termination, not by Await's error.
type Result struct {
	processID   ProcessID
	startedAt   time.Time
	finishedAt  time.Time
	output      Output
	termination Termination
	usage       Usage
}

// ProcessID returns the completed Process identity.
func (result Result) ProcessID() ProcessID { return result.processID }

// StartedAt returns the lifecycle start time.
func (result Result) StartedAt() time.Time { return result.startedAt }

// FinishedAt returns the committed terminal time.
func (result Result) FinishedAt() time.Time { return result.finishedAt }

// Status returns the terminal lifecycle state.
func (result Result) Status() Status { return result.termination.Status() }

// Termination returns the stable terminal cause and optional Failure.
func (result Result) Termination() Termination { return result.termination }

// Usage returns the final Framework-owned resource counters.
func (result Result) Usage() Usage { return result.usage }

// Output returns the final semantic result only for StatusCompleted.
func (result Result) Output() (Output, bool) { return result.output, result.output.Valid() }

// Valid reports whether the result contains one complete terminal outcome.
func (result Result) Valid() bool {
	if !result.processID.Valid() || result.startedAt.IsZero() || result.finishedAt.Before(result.startedAt) || !result.termination.Valid() {
		return false
	}
	return result.termination.Status() == StatusCompleted && result.output.Valid() ||
		result.termination.Status() != StatusCompleted && !result.output.Valid()
}

type processController struct {
	id                 ProcessID
	deployment         DeploymentRef
	relation           ProcessRelation
	childRequestDigest Digest
	budget             Budget
	capabilities       CapabilitySet
	treeLimits         TreeLimits
	startedAt          time.Time
	commands           chan processCommand
	done               chan struct{}

	viewMu      sync.RWMutex
	viewStatus  Status
	viewWaitID  WaitID
	viewUsage   Usage
	result      Result
	lastCapture Snapshot
	captureErr  error
}

func newProcessController(
	relation ProcessRelation,
	deployment DeploymentRef,
	budget Budget,
	capabilities CapabilitySet,
	treeLimits TreeLimits,
	startedAt time.Time,
	status Status,
) *processController {
	return &processController{
		id: relation.ProcessID(), deployment: deployment, relation: relation,
		budget: budget, capabilities: capabilities, treeLimits: treeLimits, startedAt: startedAt,
		commands: make(chan processCommand, 32), done: make(chan struct{}), viewStatus: status,
	}
}

// Budget returns the fixed non-renewable allocation assigned to this Process.
func (process *Process) Budget() Budget {
	if process == nil || process.controller == nil {
		return Budget{}
	}
	return process.controller.budget
}

// Capabilities returns the immutable authority set assigned to this Process.
func (process *Process) Capabilities() CapabilitySet {
	if process == nil || process.controller == nil {
		return CapabilitySet{}
	}
	return process.controller.capabilities
}

func (controller *processController) status() Status {
	controller.viewMu.RLock()
	defer controller.viewMu.RUnlock()
	return controller.viewStatus
}

func (controller *processController) waitID() (WaitID, bool) {
	controller.viewMu.RLock()
	defer controller.viewMu.RUnlock()
	return controller.viewWaitID, controller.viewStatus == StatusWaiting && controller.viewWaitID.Valid()
}

func (controller *processController) usage() Usage {
	controller.viewMu.RLock()
	defer controller.viewMu.RUnlock()
	return controller.viewUsage
}

func (controller *processController) updateView(status Status, waitID WaitID, usage Usage) {
	controller.viewMu.Lock()
	controller.viewStatus = status
	controller.viewWaitID = waitID
	controller.viewUsage = usage
	controller.viewMu.Unlock()
}

func (controller *processController) complete(result Result, snapshot Snapshot, captureErr error) {
	controller.viewMu.Lock()
	controller.viewStatus = result.Status()
	controller.viewWaitID = WaitID{}
	controller.viewUsage = result.usage
	controller.result = result
	controller.lastCapture = snapshot
	controller.captureErr = captureErr
	controller.viewMu.Unlock()
	close(controller.done)
}

func (controller *processController) terminalResult() Result {
	controller.viewMu.RLock()
	defer controller.viewMu.RUnlock()
	return controller.result
}

func (controller *processController) finishedSnapshot() (Snapshot, error, bool) {
	select {
	case <-controller.done:
		controller.viewMu.RLock()
		defer controller.viewMu.RUnlock()
		return controller.lastCapture, controller.captureErr, true
	default:
		return Snapshot{}, nil, false
	}
}

type commandKind uint8

const (
	commandInvalid commandKind = iota
	commandDeliver
	commandPause
	commandResume
	commandCancel
	commandKill
	commandResolveEffect
	commandQueryUnknownEffectIDs
	commandCapture
	commandChildrenCompleted
	commandParentTerminated
)

type processCommand struct {
	kind              commandKind
	signal            SignalRequest
	settlement        Settlement
	internalSignal    Signal
	parentTermination Termination
	reason            string
	response          chan processResponse
}

type processResponse struct {
	accepted  bool
	snapshot  Snapshot
	effectIDs []EffectID
	err       error
}

func (command processCommand) reply(response processResponse) {
	if command.response == nil {
		return
	}
	command.response <- response
}

func normalizedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
