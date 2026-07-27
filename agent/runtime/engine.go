package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

const (
	// DefaultMaxChildDepth limits recursive delegation. A root process has
	// depth zero, so this value permits eight nested child levels.
	DefaultMaxChildDepth = 8
)

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
//   - engine_run.go     — Run / Start / Continue / Resume / ResumeAsync / Kill
//   - engine_process.go — process construction + dependency wiring
//   - process_capture.go — snapshot serialization
//   - process_snapshot_tree.go — stable tree capture and removal
//   - process_restore.go — caller-supplied snapshot restoration
type Engine struct {
	catalog   deploymentRegistry // immutable deployments and active routes
	processes processRegistry    // created and restored processes

	extensions extensionRegistry // engine-scoped extensions

	events           *event.Multicast      // populated from EventListener extensions
	dependencies     *core.Dependencies    // typed engine dependency scope
	chat             core.ChatCapability   // optional shared model and streamer
	chatMiddleware   *core.ChatMiddleware  // optional shared chat middlewares
	maxToolRounds    int                   // default Prompt tool-round limit
	processMutations *processTreeSequencer // linearizes state and registry changes per process tree
	maxChildDepth    int
}

// Config is the construction-time configuration for
// [New]. A zero Config{} produces an engine with a UUID id
// generator, an in-memory blackboard, no listeners, and no tool resolvers.
// The root agent package's constructor additionally installs its default
// planners.
type Config struct {
	// Chat is the shared model capability used by managed Interact and Prompt
	// calls. Model is optional; Streamer may only be set when Model is also set.
	// Hosts can pass any implementation of the provider-neutral chat
	// interfaces. Different processes may call the shared implementations
	// concurrently; they own any synchronization required by mutable provider
	// state.
	Chat core.ChatCapability

	// ChatMiddleware is applied to every managed Interact and Prompt model
	// call. Typical uses: content safeguard, request/response logging, and
	// global quota. Optional — nil / empty means "no global wrapping".
	ChatMiddleware *core.ChatMiddleware

	// MaxToolRounds bounds the tool rounds of every managed interaction by
	// default; a process may override it. Zero leaves interactions bounded only
	// by what each states itself and by the tree Budget.
	MaxToolRounds int

	// MaxChildDepth limits recursive child-process delegation. Zero uses
	// [DefaultMaxChildDepth]; negative values are rejected.
	MaxChildDepth int

	// Extensions are the engine-scoped plug-ins. Each value must
	// implement [core.Extension] and may additionally implement any
	// subset of capability interfaces (EventListener,
	// ActionMiddleware, ToolMiddleware, AgentValidator, GoalApprover,
	// ToolGroupResolver, IDGenerator, Blackboard, planning.Planner) —
	// the runtime detects each via type assertion at dispatch time.
	//
	// [core.Extension.Name] must be unique within Extensions; an empty or
	// duplicate Name, or a value with no engine-scoped capability, makes [New]
	// return an error. Registered instances may be called concurrently as
	// described by [core.Extension].
	Extensions []core.Extension
}

// New validates config atomically and returns a fresh Engine. A
// failed construction never returns a partially initialized engine.
//
// New registers no planners — the runtime resolves them purely through the
// [planning.Planner] interface, so an agent requesting a planner (including the
// default "goap") fails at run unless a matching planner is in config.Extensions.
// Most hosts want the batteries-included composition root [agent.NewEngine],
// which installs the built-in goap/reactive planners; call New directly only to
// supply your own.
func New(config Config) (*Engine, error) {
	if config.MaxChildDepth < 0 {
		return nil, errors.New("runtime.New: MaxChildDepth must not be negative")
	}
	if config.MaxToolRounds < 0 {
		return nil, errors.New("runtime.New: MaxToolRounds must not be negative")
	}
	if valueIsNil(config.Chat.Model) && !valueIsNil(config.Chat.Streamer) {
		return nil, errors.New("runtime.New: Chat.Streamer requires Chat.Model")
	}
	chatMiddleware := cloneChatMiddleware(config.ChatMiddleware)
	maxChildDepth := config.MaxChildDepth
	if maxChildDepth == 0 {
		maxChildDepth = DefaultMaxChildDepth
	}

	engine := &Engine{
		catalog:          newDeploymentRegistry(),
		processes:        newProcessRegistry(),
		extensions:       newExtensionRegistry(),
		events:           event.NewMulticast(),
		dependencies:     core.NewDependencies(),
		chat:             config.Chat,
		chatMiddleware:   chatMiddleware,
		maxToolRounds:    config.MaxToolRounds,
		processMutations: newProcessTreeSequencer(),
		maxChildDepth:    maxChildDepth,
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

func (e *Engine) Process(id string) (*Process, bool) { return e.processes.get(id) }

// Processes returns a snapshot of all currently registered
// processes.
func (e *Engine) Processes() []*Process { return e.processes.list() }

// publishContext is the runtime's engine-scoped event entry point.
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
