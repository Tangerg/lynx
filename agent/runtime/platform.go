package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/core/model/chat"
)

// Platform is the agent runtime's top-level container — registers
// agents, builds processes, dispatches events, and exposes the
// resume API for HITL.
//
// Pluggable behavior (event listeners, action interceptors, tool
// decorators, agent validators, goal approvers, tool-group
// resolvers, id generators, planner factories, blackboard factories)
// flows through one mechanism: registered [core.Extension]s.
// Platform-scoped extensions live on [PlatformConfig.Extensions];
// per-process extensions live on [core.ProcessOptions.Extensions]
// and merge with platform extensions at dispatch time.
//
// The implementation is split across:
//
//   - platform.go         — struct + ctor + small accessors
//   - platform_deploy.go  — Deploy / Undeploy + reachability check +
//     extension-resolution fallbacks
//   - platform_run.go     — Run / Start / Continue / Resume / Kill /
//     Remove / Prune
//   - platform_process.go — createProcess / CreateChildProcess +
//     blackboard / extension wiring
type Platform struct {
	agents agentRegistry   // Deploy-time registry of *core.Agent
	procs  processRegistry // Runtime registry of *AgentProcess

	extensions extensionRegistry // platform-scoped extensions

	events       *event.Multicast      // populated from EventListener extensions
	services     *core.ServiceProvider // open registry exposed via Platform.Services()
	chatClient   *chat.Client          // optional shared LLM client
	guardrails   *core.Guardrails      // optional global chat middlewares
	processStore core.ProcessStore     // optional snapshot backend
	sessionStore core.SessionStore     // optional session persistence
	autoSnapshot bool                  // snapshot every tick when a store is configured
}

// PlatformConfig is the construction-time configuration for
// [NewPlatform]. A zero PlatformConfig{} produces a platform with
// framework defaults — UUID id generator, GOAP A* planner factory,
// in-memory blackboard, no listeners, no tool resolvers.
type PlatformConfig struct {
	// ChatClient is the shared [chat.Client] every action body
	// reaches via [core.ProcessContext.Chat] /
	// [core.ProcessContext.ChatWithActionTools]. Optional — agents
	// that don't talk to an LLM leave it nil.
	ChatClient *chat.Client

	// Guardrails are platform-wide chat middlewares applied to every
	// LLM call action bodies issue through [core.ProcessContext.Chat]
	// or [core.ProcessContext.ChatWithActionTools]. Typical uses:
	// content safeguard, request/response logging, global quota.
	// Optional — nil / empty means "no global wrapping".
	Guardrails *core.Guardrails

	// ProcessStore persists [AgentProcess] snapshots so a process
	// can survive runtime restart, be migrated across nodes, or
	// audited after termination. Optional — nil means "no
	// persistence" (lynx's historical in-memory-only behavior).
	// See [AgentProcess.Snapshot] / [Platform.RestoreProcess] /
	// [Platform.RestoreFromSnapshot] for the surface.
	ProcessStore core.ProcessStore

	// AutoSnapshot, when true and a ProcessStore is configured, makes the
	// runtime persist a snapshot after every tick (and on terminal /
	// early-termination transitions) — embabel-style automatic
	// persistence, instead of requiring an explicit [Platform.SaveProcess]
	// call. Snapshot failures are recorded on a span and do not abort the
	// running process. Ignored when ProcessStore is nil.
	AutoSnapshot bool

	// SessionStore persists multi-turn [core.Session] records so
	// conversations survive runtime restart and dispatch can pick
	// the right agent on subsequent turns. Optional — without it
	// [Platform.RunInSession] still works, but the session is not
	// saved between turns.
	SessionStore core.SessionStore

	// Extensions are the platform-scoped plug-ins. Each value must
	// implement [core.Extension] and may additionally implement any
	// subset of capability interfaces (EventListener,
	// ActionMiddleware, ToolDecorator, AgentValidator, GoalApprover,
	// ToolGroupResolver, IDGenerator, Blackboard, planning.Planner) —
	// the runtime detects each via type assertion at dispatch time.
	//
	// [core.Extension.Name] must be unique within Extensions; an
	// empty or duplicate Name causes [NewPlatform] to panic so
	// boot-time configuration errors fail fast.
	Extensions []core.Extension
}

// NewPlatform returns a fresh Platform from config. nil config is
// treated as a zero-value config (no extensions, no chat client).
// Panics on invalid extension registration (nil extension, empty
// Name, duplicate Name) — this is a deploy-time programmer error;
// callers should wire extensions correctly at boot.
func NewPlatform(config PlatformConfig) *Platform {
	p := &Platform{
		agents:       newAgentRegistry(),
		procs:        newProcessRegistry(),
		extensions:   newExtensionRegistry(),
		events:       event.NewMulticast(),
		services:     core.NewServiceProvider(),
		chatClient:   config.ChatClient,
		guardrails:   config.Guardrails,
		processStore: config.ProcessStore,
		sessionStore: config.SessionStore,
		autoSnapshot: config.AutoSnapshot,
	}
	for _, ext := range config.Extensions {
		p.extensions.register("PlatformConfig.Extensions", ext)
	}
	addEventListenerExtensions(p.events, p.extensions.list)
	return p
}

// Services exposes the platform-internal service registry — used by
// the host application to register LLM clients, RAG engines, vector
// stores, or other domain services that actions look up by key.
func (p *Platform) Services() *core.ServiceProvider { return p.services }

// NewBlackboard constructs a fresh [core.Blackboard] for a new
// process. Resolution order: a registered [core.Blackboard]
// extension (used as a prototype — Spawn() yields the isolated
// per-process instance), else the built-in in-memory implementation.
// Public so orchestration helpers — most notably the workflow
// agent-level builders — can hand a child process a clean blackboard
// rather than inheriting the parent's accumulated state via
// [core.Blackboard.Spawn].
func (p *Platform) NewBlackboard() core.Blackboard { return p.resolveBlackboard(nil) }

// Agents returns a snapshot of registered agents.
func (p *Platform) Agents() []*core.Agent { return p.agents.list() }

// FindAgent does a name lookup.
func (p *Platform) FindAgent(name string) (*core.Agent, bool) { return p.agents.find(name) }

// findAgent looks the agent up by name for agent-as-tool constructors
// ([AsChatTool] / [AsMCPTool]). Returns an error when the platform is
// nil, name is empty, or the agent isn't registered.
func (p *Platform) findAgent(label string, name string) (*core.Agent, error) {
	if p == nil {
		return nil, fmt.Errorf("runtime.%s: platform must not be nil", label)
	}
	if name == "" {
		return nil, fmt.Errorf("runtime.%s: agentName must not be empty", label)
	}
	agentDef, ok := p.FindAgent(name)
	if !ok {
		return nil, fmt.Errorf("runtime.%s: agent %q not registered on platform", label, name)
	}
	return agentDef, nil
}

// validateAgent is the [AsChatToolFromAgent] companion: same nil checks
// as [Platform.findAgent] minus the registry lookup.
func (p *Platform) validateAgent(label string, agentDef *core.Agent) error {
	if p == nil {
		return fmt.Errorf("runtime.%s: platform must not be nil", label)
	}
	if agentDef == nil {
		return fmt.Errorf("runtime.%s: agent must not be nil", label)
	}
	return nil
}

// ProcessByID looks up a process by id.
func (p *Platform) ProcessByID(id string) (*AgentProcess, bool) { return p.procs.get(id) }

// ActiveProcesses returns a snapshot of all currently registered
// processes.
func (p *Platform) ActiveProcesses() []*AgentProcess { return p.procs.list() }

// ProcessStore returns the configured snapshot backend, or nil when
// the platform was constructed without one.
func (p *Platform) ProcessStore() core.ProcessStore { return p.processStore }

// SaveProcess captures the named process into the configured
// [core.ProcessStore] under its current id. Errors when no store is
// configured, the process id is unknown, or the store rejects the
// write.
func (p *Platform) SaveProcess(ctx context.Context, processID string) error {
	if p.processStore == nil {
		return errors.New("save process: no ProcessStore configured")
	}
	proc, ok := p.procs.get(processID)
	if !ok {
		return fmt.Errorf("save process: id %q not registered", processID)
	}
	return p.processStore.Save(ctx, proc.Snapshot())
}

// RestoreProcess loads a snapshot from the configured store and
// rebuilds an [AgentProcess] bound to a currently-deployed agent
// definition. The restored process is registered in the platform's
// process map and ready for inspection or (when the snapshot status
// is resumable) re-entry into the tick loop via the standard run
// surface.
//
// Errors propagate from the store and from agent re-binding (the
// agent must be deployed under the same name as recorded in the
// snapshot).
//
// options re-attaches the per-process wiring (Extensions + Session) the
// continuation needs — see [Platform.RestoreFromSnapshot]. Pass the zero
// value for a read-only restore.
func (p *Platform) RestoreProcess(ctx context.Context, processID string, options core.ProcessOptions) (*AgentProcess, error) {
	if p.processStore == nil {
		return nil, errors.New("restore process: no ProcessStore configured")
	}
	snap, err := p.processStore.Load(ctx, processID)
	if err != nil {
		return nil, fmt.Errorf("restore process: %w", err)
	}
	return p.RestoreFromSnapshot(snap, options)
}

// publish is the runtime's event entry point. Used by AgentProcess
// and executeAction.
func (p *Platform) publish(e event.Event) {
	if e == nil {
		return
	}
	p.events.OnEvent(e)
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
