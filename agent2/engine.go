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
	// ErrInvalidEngineConfig reports incomplete execution infrastructure or
	// contradictory limits.
	ErrInvalidEngineConfig = errors.New("agent: invalid engine configuration")
	// ErrEngineClosed reports an operation attempted after Engine closure.
	ErrEngineClosed = errors.New("agent: engine is closed")
	// ErrEngineQuiescenceUnavailable reports an operation that cannot obtain a consistent
	// Engine-wide quiescent boundary.
	ErrEngineQuiescenceUnavailable = errors.New("agent: engine cannot become quiescent")
	// ErrEngineHasActiveProcesses reports closure attempted before every
	// Process reaches a terminal state.
	ErrEngineHasActiveProcesses = errors.New("agent: engine has active processes")
	// ErrProcessAlreadyExists reports a duplicate Process identity registration.
	ErrProcessAlreadyExists = errors.New("agent: process identity already exists")
)

// PreparedStepAcknowledger is the optional durability boundary immediately
// before external Effect dispatch. Returning nil confirms only that this exact
// Snapshot reached the caller's chosen durable boundary; it does not grant the
// Framework ownership of the caller's persistence or atomicity semantics.
type PreparedStepAcknowledger interface {
	AcknowledgePreparedStep(ctx context.Context, snapshot Snapshot) error
}

// EngineConfig contains only cross-Strategy execution mechanics. Definition,
// Dispatcher, schema, and behavior configuration belong to each Deployment.
type EngineConfig struct {
	// PreparedStepAcknowledger enables the optional pre-dispatch durability
	// handshake. Implementations may be called concurrently for different
	// Processes, must return in bounded time, and must not re-enter the Process
	// represented by the supplied Snapshot.
	PreparedStepAcknowledger PreparedStepAcknowledger

	// DeploymentResolver supplies exact Deployments requested by child Effects
	// and tree restoration. It is unnecessary for same-Deployment recursion.
	// Resolution is a bounded, context-free local binding lookup only; the
	// resolver does not perform routing or own Process construction or lifecycle.
	DeploymentResolver DeploymentResolver

	// ProcessAdmitter is the optional decision boundary immediately before any
	// root or child Process starts. It observes immutable Framework facts and
	// may reject, but cannot modify resource or capability allocation.
	ProcessAdmitter ProcessAdmitter

	// EventListeners receive ordered facts for each Process. Different
	// Processes may call a listener concurrently.
	EventListeners []EventListener

	// DeltaListeners receive best-effort streaming increments from the shared
	// bounded queue. Delivery to each listener is sequential.
	DeltaListeners []DeltaListener

	// DeltaBufferCapacity bounds the Engine-wide pending Delta queue. Zero uses
	// the documented internal default; negative values are invalid.
	DeltaBufferCapacity int

	// Limits supplies per-Process execution bounds. Each zero field inherits
	// the corresponding value from DefaultLimits.
	Limits Limits

	// TreeLimits bounds child depth, lifetime fan-out, active children, and the
	// total Process count in each independent tree.
	TreeLimits TreeLimits

	// Capabilities is the maximum authority of each root Process. Child Effects
	// may only allocate subsets and Dispatcher Effects declare what they require.
	Capabilities CapabilitySet
}

// Engine is the sole owner of Process construction, scheduling, lifecycle,
// Signal delivery, Effect dispatch, and snapshot boundaries. It contains no
// Deployment catalog or Host persistence abstraction.
type Engine struct {
	acknowledger    PreparedStepAcknowledger
	resolver        DeploymentResolver
	admitter        ProcessAdmitter
	observation     *observationBus
	limits          Limits
	treeLimits      TreeLimits
	capabilities    CapabilitySet
	treeOperationMu sync.Mutex

	mu         sync.RWMutex
	processes  map[ProcessID]*processController
	children   map[childIdentity]ProcessID
	childWaits map[WaitID]*childWaitRegistration
	closed     bool
}

// NewEngine validates execution infrastructure and returns an empty Engine.
func NewEngine(config EngineConfig) (*Engine, error) {
	if config.DeltaBufferCapacity < 0 {
		return nil, fmt.Errorf("%w: DeltaBufferCapacity must not be negative", ErrInvalidEngineConfig)
	}
	if !nilOrConcrete(config.PreparedStepAcknowledger) {
		return nil, fmt.Errorf("%w: PreparedStepAcknowledger is typed nil", ErrInvalidEngineConfig)
	}
	if !nilOrConcrete(config.DeploymentResolver) {
		return nil, fmt.Errorf("%w: DeploymentResolver is typed nil", ErrInvalidEngineConfig)
	}
	if !nilOrConcrete(config.ProcessAdmitter) {
		return nil, fmt.Errorf("%w: ProcessAdmitter is typed nil", ErrInvalidEngineConfig)
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
	treeLimits, err := effectiveTreeLimits(config.TreeLimits)
	if err != nil {
		return nil, fmt.Errorf("%w: tree limits are invalid", ErrInvalidEngineConfig)
	}
	if !config.Capabilities.Valid() {
		return nil, fmt.Errorf("%w: capabilities are invalid", ErrInvalidEngineConfig)
	}
	return &Engine{
		acknowledger: config.PreparedStepAcknowledger,
		resolver:     config.DeploymentResolver,
		admitter:     config.ProcessAdmitter,
		observation:  newObservationBus(config.EventListeners, config.DeltaListeners, capacity),
		limits:       limits,
		treeLimits:   treeLimits,
		capabilities: config.Capabilities,
		processes:    make(map[ProcessID]*processController),
		children:     make(map[childIdentity]ProcessID),
		childWaits:   make(map[WaitID]*childWaitRegistration),
	}, nil
}

type childIdentity struct {
	parent ProcessID
	key    ChildKey
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
	id, err := newProcessID()
	if err != nil {
		return nil, err
	}
	relation := rootProcessRelation(id)
	budget := budgetFromLimits(engine.limits)
	admission := newProcessAdmission(relation, deployment, budget, engine.capabilities)
	if err := requestProcessAdmission(engine.admitter, admission); err != nil {
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
	startedAt := time.Now().Round(0).UTC()
	controller := newProcessController(
		relation, deployment.DeploymentRef(), budget, engine.capabilities,
		engine.treeLimits,
		startedAt, StatusRunning,
	)
	loop := newProcessLoop(engine, controller, deployment, execution, state, startedAt, engine.limits)
	if err := engine.register(controller); err != nil {
		return nil, err
	}
	go loop.run(contextOrBackground(ctx))
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
	return process.Await(context.WithoutCancel(contextOrBackground(ctx)))
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
	controller, loop, wire, err := prepareRestoredProcess(engine, deployment, snapshot)
	if err != nil {
		return nil, err
	}
	if !controller.relation.IsRoot() || wire.ReservedBudget != (Budget{}) || hasOpenChildWait(wire.Mailbox) {
		return nil, ErrTreeSnapshotRequired
	}
	if err := engine.register(controller); err != nil {
		return nil, err
	}
	if wire.Status.Terminal() {
		controller.complete(loop.result(), snapshot, nil)
		controller.markTreeSettled()
		return &Process{controller: controller}, nil
	}
	go loop.run(contextOrBackground(ctx))
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
			return ErrEngineHasActiveProcesses
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
	if _, exists := engine.processes[controller.processID]; exists {
		return ErrProcessAlreadyExists
	}
	engine.processes[controller.processID] = controller
	return nil
}

func (engine *Engine) registerChild(controller *processController, requestDigest Digest) error {
	relation := controller.relation
	parentID, child := relation.ParentID()
	key, keyed := relation.ChildKey()
	if !child || !keyed || !requestDigest.Valid() {
		return ErrInvalidProcessRelation
	}
	identity := childIdentity{parent: parentID, key: key}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.closed {
		return ErrEngineClosed
	}
	if existingID, exists := engine.children[identity]; exists {
		existing := engine.processes[existingID]
		if existing != nil && existing.processID == controller.processID &&
			existing.deploymentRef == controller.deploymentRef && existing.childRequestDigest == requestDigest {
			return nil
		}
		return ErrInvalidChildStart
	}
	if _, exists := engine.processes[controller.processID]; exists {
		return ErrProcessAlreadyExists
	}
	if _, exists := engine.processes[parentID]; !exists {
		return ErrInvalidProcessRelation
	}
	parent := engine.processes[parentID]
	if parent == nil || controller.treeLimits != parent.treeLimits ||
		relation.depth > controller.treeLimits.MaxDepth {
		return ErrResourceLimitExceeded
	}
	var childCount, activeChildCount, treeProcessCount uint32
	for _, existing := range engine.processes {
		if existing.relation.rootID == relation.rootID {
			treeProcessCount++
		}
		if existing.relation.parentID == parentID {
			childCount++
			if !existing.status().Terminal() {
				activeChildCount++
			}
		}
	}
	if childCount >= controller.treeLimits.MaxChildren ||
		activeChildCount >= controller.treeLimits.MaxActiveChildren ||
		treeProcessCount >= controller.treeLimits.MaxTreeProcesses {
		return ErrResourceLimitExceeded
	}
	controller.childRequestDigest = requestDigest
	engine.processes[controller.processID] = controller
	engine.children[identity] = controller.processID
	return nil
}

func newProcessID() (ProcessID, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ProcessID{}, fmt.Errorf("agent: generate ProcessID: %w", err)
	}
	return ParseProcessID("process:" + hex.EncodeToString(random[:]))
}
