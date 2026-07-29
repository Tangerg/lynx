package runtime

import (
	"cmp"
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

const (
	// DefaultMaxChildDepth bounds recursive delegation, counting a root process
	// as depth zero. Unlike cost or round limits this is a framework rail rather
	// than host policy: every level keeps another registered process alive in the
	// tree, so an unbounded default would be a failure mode. Config can move it.
	DefaultMaxChildDepth = 8
)

// Engine is the agent runtime's top-level container — registers
// agents, builds processes, dispatches events, and exposes the
// resume API for HITL.
//
// Pluggable behavior flows through one mechanism: registered
// [core.Extension]s, enumerated on [Config.Extensions]. Engine-scoped ones live
// there; per-process ones live on [core.ProcessOptions.Extensions] and merge
// with the engine set at dispatch time.
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
	// call. Optional — nil / empty means "no global wrapping".
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
	// ToolGroupResolver, ChildAdmitter, IDGenerator, Blackboard,
	// planning.Planner) — the runtime detects each via type assertion at
	// dispatch time.
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
	maxChildDepth := cmp.Or(config.MaxChildDepth, DefaultMaxChildDepth)

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

// NewBlackboard constructs a fresh [core.Blackboard] for a process that will
// run agent. Resolution order: a registered [core.Blackboard] extension (used
// as a prototype — Clone yields the isolated per-process instance), else the
// built-in in-memory implementation.
//
// Public so orchestration helpers — most notably the workflow agent-level
// builders — can hand a child process a clean blackboard rather than inheriting
// the parent's accumulated state via Clone. agent is required because its
// declared snapshot state is what the returned blackboard admits: a blackboard
// built for one agent must not be handed to a process that cannot restore what
// it holds. It returns an error when a registered prototype panics or violates
// the Clone contract.
func (e *Engine) NewBlackboard(agent *core.Agent) (core.Blackboard, error) {
	if agent == nil {
		return nil, errors.New("runtime.Engine.NewBlackboard: agent is nil")
	}
	return e.resolveBlackboard(agent.SnapshotCodec(), nil)
}

// Process returns the live process registered under id. It is the process
// itself, not a copy, so an observer sees state advance as the tick advances it.
// A false result does not distinguish "never registered" from "already removed";
// either way there is nothing to act on.
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
