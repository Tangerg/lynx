package agent2

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrInvalidEngineConfig = errors.New("agent: invalid Engine configuration")
	ErrEngineClosed        = errors.New("agent: Engine is closed")
	ErrEngineBusy          = errors.New("agent: Engine has active Processes")
	ErrProcessExists       = errors.New("agent: Process identity already exists")
)

// PreparedStepAcknowledger is the optional durability boundary immediately
// before external Effect dispatch. Returning nil confirms only that this exact
// Snapshot reached the caller's chosen durable boundary; it does not grant the
// Framework ownership of a Store, transaction, or application write set.
type PreparedStepAcknowledger interface {
	AcknowledgePreparedStep(context.Context, Snapshot) error
}

// EngineConfig contains only cross-Strategy execution mechanics. Definition,
// Dispatcher, schema, and behavior configuration belong to each Deployment.
type EngineConfig struct {
	PreparedStepAcknowledger PreparedStepAcknowledger
	EventListeners           []EventListener
	DeltaListeners           []DeltaListener
	DeltaBufferCapacity      int
	Limits                   Limits
}

// Engine is the sole owner of Process construction, scheduling, lifecycle,
// Signal delivery, Effect dispatch, and snapshot boundaries. It contains no
// Deployment catalog or Host persistence abstraction.
type Engine struct {
	acknowledger PreparedStepAcknowledger
	observation  *observationBus
	limits       Limits

	mu        sync.RWMutex
	processes map[ProcessID]*processController
	closed    bool
}

// NewEngine validates execution infrastructure and returns an empty Engine.
func NewEngine(config EngineConfig) (*Engine, error) {
	if config.DeltaBufferCapacity < 0 {
		return nil, fmt.Errorf("%w: DeltaBufferCapacity must not be negative", ErrInvalidEngineConfig)
	}
	if !nilOrConcrete(config.PreparedStepAcknowledger) {
		return nil, fmt.Errorf("%w: PreparedStepAcknowledger is typed nil", ErrInvalidEngineConfig)
	}
	for index, listener := range config.EventListeners {
		if nilInterface(listener) {
			return nil, fmt.Errorf("%w: EventListeners[%d] is nil", ErrInvalidEngineConfig, index)
		}
	}
	for index, listener := range config.DeltaListeners {
		if nilInterface(listener) {
			return nil, fmt.Errorf("%w: DeltaListeners[%d] is nil", ErrInvalidEngineConfig, index)
		}
	}
	capacity := config.DeltaBufferCapacity
	if capacity == 0 {
		capacity = defaultDeltaBuffer
	}
	limits, err := effectiveLimits(config.Limits)
	if err != nil {
		return nil, fmt.Errorf("%w: limits are invalid", ErrInvalidEngineConfig)
	}
	return &Engine{
		acknowledger: config.PreparedStepAcknowledger,
		observation:  newObservationBus(config.EventListeners, config.DeltaListeners, capacity),
		limits:       limits,
		processes:    make(map[ProcessID]*processController),
	}, nil
}

func nilOrConcrete(value any) bool { return value == nil || !nilInterface(value) }

// Start validates Input, creates exactly one Execution, registers its Process,
// and starts the Engine-owned loop. Cancelling ctx records a Host cancellation
// or deadline; use a longer-lived context for execution beyond a request.
func (engine *Engine) Start(ctx context.Context, deployment Deployment, input Input) (*Process, error) {
	if engine == nil {
		return nil, ErrInvalidEngineConfig
	}
	if !deployment.Valid() {
		return nil, ErrInvalidDeployment
	}
	if err := deployment.Descriptor().ValidateInput(input); err != nil {
		return nil, err
	}
	execution, err := startExecution(deployment.Definition(), input)
	if err != nil {
		return nil, fmt.Errorf("agent: start Execution: %w", err)
	}
	state, err := captureExecution(execution)
	if err != nil {
		return nil, fmt.Errorf("agent: capture initial Execution state: %w", err)
	}
	execution, err = restoreExecution(deployment.Definition(), state)
	if err != nil {
		return nil, fmt.Errorf("agent: validate initial Execution state: %w", err)
	}
	id, err := newProcessID()
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().Round(0).UTC()
	controller := newProcessController(id, deployment.Reference(), startedAt, StatusRunning)
	runtime := newProcessRuntime(engine, controller, deployment, execution, state, startedAt, engine.limits)
	if err := engine.register(controller); err != nil {
		return nil, err
	}
	go runtime.run(normalizedContext(ctx))
	return &Process{controller: controller}, nil
}

// Run starts one Process and waits for its terminal result. Once Start succeeds,
// Run waits for safe finalization even if ctx is cancelled; the same ctx has
// already recorded the Process termination intent.
func (engine *Engine) Run(ctx context.Context, deployment Deployment, input Input) (Result, error) {
	process, err := engine.Start(ctx, deployment, input)
	if err != nil {
		return Result{}, err
	}
	return process.Await(context.WithoutCancel(normalizedContext(ctx)))
}

// Restore recreates one Process from a strict Snapshot and the exact bound
// Deployment. A different implementation or configuration digest is rejected.
func (engine *Engine) Restore(ctx context.Context, deployment Deployment, snapshot Snapshot) (*Process, error) {
	if engine == nil {
		return nil, ErrInvalidEngineConfig
	}
	if !deployment.Valid() {
		return nil, ErrInvalidDeployment
	}
	wire, err := snapshot.wire()
	if err != nil {
		return nil, err
	}
	if wire.Deployment != deployment.Reference() {
		return nil, fmt.Errorf("%w: exact Deployment does not match", ErrInvalidSnapshot)
	}
	execution, err := restoreExecution(deployment.Definition(), wire.LastStable)
	if err != nil {
		return nil, fmt.Errorf("%w: restore Execution: %w", ErrInvalidSnapshot, err)
	}
	mailbox, err := restoreSignalMailbox(wire.Mailbox)
	if err != nil {
		return nil, fmt.Errorf("%w: mailbox: %w", ErrInvalidSnapshot, err)
	}
	controller := newProcessController(wire.ProcessID, wire.Deployment, wire.StartedAt, wire.Status)
	runtime, err := restoreProcessRuntime(engine, controller, deployment, execution, mailbox, wire)
	if err != nil {
		return nil, err
	}
	if err := engine.register(controller); err != nil {
		return nil, err
	}
	if wire.Status.Terminal() {
		controller.complete(runtime.result(), snapshot, nil)
		return &Process{controller: controller}, nil
	}
	go runtime.run(normalizedContext(ctx))
	return &Process{controller: controller}, nil
}

// Process returns an Engine-issued handle for an identity known to this Engine.
func (engine *Engine) Process(id ProcessID) (*Process, bool) {
	if engine == nil || !id.Valid() {
		return nil, false
	}
	engine.mu.RLock()
	controller, exists := engine.processes[id]
	engine.mu.RUnlock()
	if !exists {
		return nil, false
	}
	return &Process{controller: controller}, true
}

// Close releases observation workers after all Processes have reached a
// terminal state. Process results remain readable from existing handles.
func (engine *Engine) Close() error {
	if engine == nil {
		return nil
	}
	engine.mu.Lock()
	if engine.closed {
		engine.mu.Unlock()
		return nil
	}
	for _, controller := range engine.processes {
		if !controller.status().Terminal() {
			engine.mu.Unlock()
			return ErrEngineBusy
		}
	}
	engine.closed = true
	engine.mu.Unlock()
	engine.observation.close()
	return nil
}

func (engine *Engine) register(controller *processController) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.closed {
		return ErrEngineClosed
	}
	if _, exists := engine.processes[controller.id]; exists {
		return ErrProcessExists
	}
	engine.processes[controller.id] = controller
	return nil
}

func newProcessID() (ProcessID, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ProcessID{}, fmt.Errorf("agent: generate ProcessID: %w", err)
	}
	return ParseProcessID("process:" + hex.EncodeToString(random[:]))
}
