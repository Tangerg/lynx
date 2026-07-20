package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// DefaultSessionFinalizeTimeout bounds the durable session write performed
// after a turn finishes. It is independent of request cancellation.
const DefaultSessionFinalizeTimeout = 10 * time.Second

// Engine is the agent runtime's top-level container — registers
// agents, builds processes, dispatches events, and exposes the
// resume API for HITL.
//
// Pluggable behavior (event listeners, action and tool middleware,
// agent validators, goal approvers, tool-group resolvers, id generators,
// planners, and blackboard prototypes)
// flows through one mechanism: registered [core.Extension]s.
// Engine-scoped extensions live on [Config.Extensions];
// per-process extensions live on [core.ProcessOptions.Extensions]
// and merge with engine extensions at dispatch time.
//
// The implementation is split across:
//
//   - engine.go         — struct + constructor + small accessors
//   - engine_deploy.go  — Deploy / Undeploy + reachability check +
//     extension-resolution fallbacks
//   - engine_run.go     — Run / Start / Continue / Resume / Kill /
//     Remove / Prune
//   - engine_process.go — process construction + dependency wiring
type Engine struct {
	catalog   deploymentRegistry // immutable deployments and active routes
	processes processRegistry    // created and restored processes

	extensions extensionRegistry // engine-scoped extensions

	events                 *event.Multicast     // populated from EventListener extensions
	dependencies           *core.Dependencies   // typed engine dependency scope
	chat                   core.ChatCapability  // optional shared model and streamer
	guardrails             *core.ChatGuardrails // optional global chat middlewares
	processStore           core.ProcessStore    // optional snapshot backend
	sessionStore           core.SessionStore    // optional root-session persistence
	childSessionStore      core.SessionStore    // optional delegated-session persistence
	sessionTurnSequencer   SessionTurnSequencer // orders turns sharing a session ID
	sessionFinalizeTimeout time.Duration        // bounds the post-dispatch session write
	autoSnapshot           bool                 // snapshot every tick when a store is configured
	snapshotFailurePolicy  SnapshotFailurePolicy
	buildID                string // stable host build identity included in deployment digests
}

// Config is the construction-time configuration for
// [New]. A zero Config{} produces an engine with a UUID id
// generator, an in-memory blackboard, no listeners, and no tool resolvers.
// The root agent package's constructor additionally installs its default
// planners.
type Config struct {
	// BuildID is a stable host/application build identity included in every
	// deployment digest. An engine with ProcessStore requires each Agent to
	// provide an explicit semantic Version or this field to be non-empty.
	BuildID string

	// Chat is the shared model capability every action body reaches through
	// [core.ProcessContext.Chat] or [core.ProcessContext.Prompt]. Model is
	// optional; Streamer may only be set when Model is also set. Hosts can pass
	// any implementation of the provider-neutral chat interfaces.
	Chat core.ChatCapability

	// Guardrails are engine-wide chat middlewares applied to every
	// LLM call action bodies issue through [core.ProcessContext.Chat]
	// or [core.ProcessContext.Prompt]. Typical uses:
	// content safeguard, request/response logging, global quota.
	// Optional — nil / empty means "no global wrapping".
	Guardrails *core.ChatGuardrails

	// ProcessStore persists [Process] snapshots so a process
	// can survive a single-owner runtime restart or be audited after
	// termination. CAS prevents lost updates but does not elect an execution
	// owner; cross-node handoff requires Host-provided lease/fencing outside
	// this contract. Optional — nil means "no
	// persistence" (historical in-memory-only behavior).
	// See [Process.Snapshot] / [Engine.Restore] /
	// [Engine.RestoreSnapshot] for the surface.
	ProcessStore core.ProcessStore

	// AutoSnapshot, when true and a ProcessStore is configured, makes the
	// runtime persist a snapshot after every tick (and on terminal /
	// early-termination transitions) — automatic persistence, instead of
	// requiring an explicit [Engine.Save] call. Snapshot failures
	// follow SnapshotFailurePolicy. Ignored when ProcessStore is nil.
	AutoSnapshot bool

	// SnapshotFailurePolicy decides what an automatic snapshot failure does.
	// The zero value fails the run. Pause keeps a non-terminal process resumable;
	// ReportOnly emits a degradation event and continues explicitly non-durable.
	SnapshotFailurePolicy SnapshotFailurePolicy

	// SessionStore persists multi-turn [core.Session] records so
	// conversations survive runtime restart and dispatch can pick
	// the right agent on subsequent turns. Optional — without it
	// [Engine.RunInSession] still works, but the session is not
	// saved between turns.
	SessionStore core.SessionStore

	// SessionTurnSequencer orders concurrent turns for the same session ID. The
	// default is process-local and still allows different sessions to run in
	// parallel. Multi-node hosts should provide a distributed implementation.
	SessionTurnSequencer SessionTurnSequencer

	// SessionFinalizeTimeout bounds the post-dispatch SessionStore write. That
	// write is detached from request cancellation so audit state can survive a
	// canceled request, but it must not hold the session turn forever. Zero uses
	// [DefaultSessionFinalizeTimeout]; negative values are rejected.
	SessionFinalizeTimeout time.Duration

	// ChildSessionStore persists sessions created for delegated child
	// processes. It is separate from SessionStore because hosts may use a
	// product-specific lineage backend that must never receive root-session
	// writes. Configure the same backend in both fields when one store owns both
	// lifecycles.
	ChildSessionStore core.SessionStore

	// Extensions are the engine-scoped plug-ins. Each value must
	// implement [core.Extension] and may additionally implement any
	// subset of capability interfaces (EventListener,
	// ActionMiddleware, ToolMiddleware, AgentValidator, GoalApprover,
	// ToolGroupResolver, IDGenerator, Blackboard, planning.Planner) —
	// the runtime detects each via type assertion at dispatch time.
	//
	// [core.Extension.Name] must be unique within Extensions; an empty or
	// duplicate Name makes [New] return an error.
	Extensions []core.Extension
}

// New validates config atomically and returns a fresh Engine. A
// failed construction never returns a partially initialized engine.
func New(config Config) (*Engine, error) {
	if config.BuildID != strings.TrimSpace(config.BuildID) {
		return nil, errors.New("runtime.New: BuildID must not have leading or trailing whitespace")
	}
	if config.AutoSnapshot && valueIsNil(config.ProcessStore) {
		return nil, errors.New("runtime.New: AutoSnapshot requires ProcessStore")
	}
	if !config.SnapshotFailurePolicy.Valid() {
		return nil, errors.New("runtime.New: invalid SnapshotFailurePolicy")
	}
	if config.ProcessStore != nil && valueIsNil(config.ProcessStore) {
		return nil, errors.New("runtime.New: ProcessStore is typed nil")
	}
	if config.SessionStore != nil && valueIsNil(config.SessionStore) {
		return nil, errors.New("runtime.New: SessionStore is typed nil")
	}
	if config.SessionTurnSequencer != nil && valueIsNil(config.SessionTurnSequencer) {
		return nil, errors.New("runtime.New: SessionTurnSequencer is typed nil")
	}
	if config.SessionFinalizeTimeout < 0 {
		return nil, errors.New("runtime.New: SessionFinalizeTimeout must not be negative")
	}
	if config.ChildSessionStore != nil && valueIsNil(config.ChildSessionStore) {
		return nil, errors.New("runtime.New: ChildSessionStore is typed nil")
	}
	if valueIsNil(config.Chat.Model) && !valueIsNil(config.Chat.Streamer) {
		return nil, errors.New("runtime.New: Chat.Streamer requires Chat.Model")
	}
	guardrails, err := snapshotChatGuardrails("runtime.New: Guardrails", config.Guardrails)
	if err != nil {
		return nil, err
	}
	turnSequencer := config.SessionTurnSequencer
	if turnSequencer == nil {
		turnSequencer = newLocalSessionTurnSequencer()
	}
	finalizeTimeout := config.SessionFinalizeTimeout
	if finalizeTimeout == 0 {
		finalizeTimeout = DefaultSessionFinalizeTimeout
	}

	engine := &Engine{
		catalog:                newDeploymentRegistry(),
		processes:              newProcessRegistry(),
		extensions:             newExtensionRegistry(),
		events:                 event.NewMulticast(),
		dependencies:           core.NewDependencies(),
		chat:                   config.Chat,
		guardrails:             guardrails,
		processStore:           config.ProcessStore,
		sessionStore:           config.SessionStore,
		childSessionStore:      config.ChildSessionStore,
		sessionTurnSequencer:   turnSequencer,
		sessionFinalizeTimeout: finalizeTimeout,
		autoSnapshot:           config.AutoSnapshot,
		snapshotFailurePolicy:  config.SnapshotFailurePolicy,
		buildID:                config.BuildID,
	}
	for _, extension := range config.Extensions {
		if err := engine.extensions.register("Config.Extensions", extension); err != nil {
			return nil, err
		}
	}
	addEventListenerExtensions(engine.events, engine.extensions.list)
	return engine, nil
}

// MustNew is the startup/test companion to New. It panics with
// the original validation error and should not be used for dynamic host input.
func MustNew(config Config) *Engine {
	engine, err := New(config)
	if err != nil {
		panic(err)
	}
	return engine
}

// Dependencies exposes the typed engine dependency scope. Hosts register dynamic
// domain dependencies during composition; the scope freezes when the first process
// starts. Build per-process overrides with Dependencies().Child() and pass that
// child through [core.ProcessOptions.Dependencies].
func (e *Engine) Dependencies() *core.Dependencies { return e.dependencies }

// NewBlackboard constructs a fresh [core.Blackboard] for a new
// process. Resolution order: a registered [core.Blackboard]
// extension (used as a prototype — Clone() yields the isolated
// per-process instance), else the built-in in-memory implementation.
// Public so orchestration helpers — most notably the workflow
// agent-level builders — can hand a child process a clean blackboard
// rather than inheriting the parent's accumulated state via
// [core.Blackboard.Clone].
func (e *Engine) NewBlackboard() core.Blackboard { return e.resolveBlackboard(nil) }

// findDeployment looks the active deployment up by name for agent-as-tool constructors
// ([NewAgentTool] / [NewStandaloneAgentTool]). Returns an error when the engine is
// nil, name is empty, or the agent isn't registered.
func (e *Engine) findDeployment(label string, name string) (*Deployment, error) {
	if e == nil {
		return nil, fmt.Errorf("runtime.%s: engine must not be nil", label)
	}
	if name == "" {
		return nil, fmt.Errorf("runtime.%s: agentName must not be empty", label)
	}
	deployment, ok := e.catalog.activeDeployment(name)
	if !ok {
		return nil, fmt.Errorf("runtime.%s: agent %q not registered on engine", label, name)
	}
	return deployment, nil
}

func (e *Engine) Process(id string) (*Process, bool) { return e.processes.get(id) }

// Processes returns a snapshot of all currently registered
// processes.
func (e *Engine) Processes() []*Process { return e.processes.list() }

// ProcessStore returns the configured snapshot backend, or nil when
// the engine was constructed without one.
func (e *Engine) ProcessStore() core.ProcessStore { return e.processStore }

// Save captures the named process into the configured
// [core.ProcessStore] under its current id. Errors when no store is
// configured, the process id is unknown, or the store rejects the
// write.
func (e *Engine) Save(ctx context.Context, processID string) (uint64, error) {
	if e.processStore == nil {
		return 0, errors.New("runtime.Engine.Save: no ProcessStore configured")
	}
	process, ok := e.processes.get(processID)
	if !ok {
		return 0, fmt.Errorf("runtime.Engine.Save: id %q not registered", processID)
	}
	return e.saveProcess(ctx, process)
}

func (e *Engine) saveProcess(ctx context.Context, process *Process) (uint64, error) {
	if e.processStore == nil {
		return 0, errors.New("runtime.Engine.saveProcess: no ProcessStore configured")
	}
	ctx = normalizeContext(ctx)
	tree, err := e.lockProcessTree(process, map[string]struct{}{})
	if err != nil {
		return 0, err
	}

	if err := captureLockedProcessTree(tree); err != nil {
		unlockProcessTree(tree)
		return 0, err
	}
	revision, err := e.saveLockedProcessTree(ctx, tree)
	if err != nil {
		unlockProcessTree(tree)
		return 0, err
	}
	var cleanup []string
	collectNestedChildCleanup(tree, &cleanup)
	unlockProcessTree(tree)
	for _, childID := range cleanup {
		e.discardProcessTree(ctx, childID)
	}
	return revision, nil
}

type lockedProcessTree struct {
	process   *Process
	relations []*nestedChildRelation
	children  []*lockedProcessTree
	snapshot  core.ProcessSnapshot
}

func (e *Engine) lockProcessTree(process *Process, visited map[string]struct{}) (*lockedProcessTree, error) {
	if process == nil {
		return nil, errors.New("runtime.Engine.saveProcess: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return nil, fmt.Errorf("%w: nested process cycle at %q", core.ErrInvalidSnapshot, process.ID())
	}
	visited[process.ID()] = struct{}{}
	process.checkpointMu.Lock()
	checkpoint, err := nestedChildrenFromSuspension(process.Suspension())
	if err != nil {
		process.checkpointMu.Unlock()
		return nil, err
	}

	tree := &lockedProcessTree{
		process:   process,
		relations: checkpoint.relations,
		children:  make([]*lockedProcessTree, 0, len(checkpoint.relations)),
	}
	for _, relation := range checkpoint.relations {
		child, ok := e.Process(relation.ChildID)
		if !ok {
			unlockProcessTree(tree)
			return nil, fmt.Errorf("%w: nested child process %q is missing", core.ErrInvalidSnapshot, relation.ChildID)
		}
		childTree, lockErr := e.lockProcessTree(child, visited)
		if lockErr != nil {
			unlockProcessTree(tree)
			return nil, fmt.Errorf("lock nested child %q: %w", child.ID(), lockErr)
		}
		tree.children = append(tree.children, childTree)
		if err := relation.validateProcess(process, child); err != nil {
			unlockProcessTree(tree)
			return nil, err
		}
	}
	return tree, nil
}

func captureLockedProcessTree(tree *lockedProcessTree) error {
	if tree == nil || tree.process == nil {
		return errors.New("runtime.Engine.saveProcess: locked process tree is incomplete")
	}
	snapshot, err := tree.process.snapshot()
	if err != nil {
		return err
	}
	tree.snapshot = snapshot
	for index, child := range tree.children {
		if err := captureLockedProcessTree(child); err != nil {
			return err
		}
		if err := tree.relations[index].validateSnapshot(tree.snapshot, child.snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) saveLockedProcessTree(ctx context.Context, tree *lockedProcessTree) (uint64, error) {
	for _, child := range tree.children {
		if _, err := e.saveLockedProcessTree(ctx, child); err != nil {
			return 0, err
		}
	}
	snapshot := tree.snapshot
	revision, err := e.processStore.Save(ctx, snapshot, snapshot.Revision)
	if err != nil {
		return 0, err
	}
	if !tree.process.state.commitRevision(snapshot.Revision, revision) {
		return 0, &core.RevisionConflictError{
			ProcessID: tree.process.ID(),
			Expected:  snapshot.Revision,
			Actual:    tree.process.state.snapshotRevision(),
		}
	}
	return revision, nil
}

func collectNestedChildCleanup(tree *lockedProcessTree, cleanup *[]string) {
	if tree == nil {
		return
	}
	*cleanup = append(*cleanup, tree.process.takeNestedChildCleanup()...)
	for _, child := range tree.children {
		collectNestedChildCleanup(child, cleanup)
	}
}

func unlockProcessTree(tree *lockedProcessTree) {
	if tree == nil {
		return
	}
	for index := len(tree.children) - 1; index >= 0; index-- {
		unlockProcessTree(tree.children[index])
	}
	tree.process.checkpointMu.Unlock()
}

// Restore loads a snapshot from the configured store and
// rebuilds an [Process] bound to a currently-deployed agent
// definition. The restored process is registered in the engine's
// process map and ready for inspection or (when the snapshot status
// is resumable) re-entry into the tick loop via the standard run
// surface.
//
// Errors propagate from the store and from agent re-binding (the
// agent must be deployed under the same name as recorded in the
// snapshot).
//
// options re-attaches the per-process wiring (Extensions + Session) the
// continuation needs — see [Engine.RestoreSnapshot]. Pass the zero
// value for a read-only restore.
func (e *Engine) Restore(ctx context.Context, processID string, options core.ProcessOptions) (*Process, error) {
	if e.processStore == nil {
		return nil, errors.New("runtime.Engine.Restore: no ProcessStore configured")
	}
	snapshot, err := e.processStore.Load(ctx, processID)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.Restore: %w", err)
	}
	return e.RestoreSnapshot(snapshot, options)
}

// publish is the runtime's event entry point. Used by Process
// and executeAction.
func (e *Engine) publish(published event.Event) {
	e.publishContext(context.Background(), published)
}

func (e *Engine) publishContext(ctx context.Context, published event.Event) {
	if published == nil {
		return
	}
	e.events.OnEvent(ctx, published)
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
