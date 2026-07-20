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
//   - process_snapshot.go — snapshot capture, persistence, and restoration
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
	sessionTurnSequencer   SessionTurnSequencer // sequences turns sharing a session ID
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

	// SessionTurnSequencer grants turns for the same session ID in arrival order.
	// The default is process-local and still allows different sessions to run in
	// parallel. Cross-node execution additionally requires Host-owned fencing;
	// this interface alone cannot reject a stale owner.
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
// [core.Blackboard.Clone]. It returns an error when a registered prototype
// panics or violates the Clone contract.
func (e *Engine) NewBlackboard() (core.Blackboard, error) { return e.resolveBlackboard(nil) }

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
