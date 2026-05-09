package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/core/model/chat"
)

// Platform is the agent runtime's top-level container — registers
// agents, builds processes, dispatches events, and exposes the
// resume API for HITL.
//
// Pluggable behaviour (event listeners, action interceptors, tool
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

	events     *event.Multicast      // populated from EventListener extensions
	services   *core.ServiceProvider // open registry exposed via Platform.Services()
	chatClient *chat.Client          // optional shared LLM client
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

	// Extensions are the platform-scoped plug-ins. Each value must
	// implement [core.Extension] and may additionally implement any
	// subset of capability interfaces (EventListener,
	// ActionInterceptor, ToolDecorator, AgentValidator, GoalApprover,
	// ToolGroupResolver, IDGenerator, PlannerFactory,
	// BlackboardFactory) — the runtime detects each via type
	// assertion at dispatch time.
	//
	// [core.Extension.Name] must be unique within Extensions; an
	// empty or duplicate Name causes [NewPlatform] to panic so
	// boot-time configuration errors fail fast.
	Extensions []core.Extension
}

// NewPlatform returns a fresh Platform from config. Panics on
// invalid extension registration (nil extension, empty Name,
// duplicate Name).
func NewPlatform(config PlatformConfig) *Platform {
	p := &Platform{
		agents:     newAgentRegistry(),
		procs:      newProcessRegistry(),
		extensions: newExtensionRegistry(),
		events:     event.NewMulticast(),
		services:   core.NewServiceProvider(),
		chatClient: config.ChatClient,
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

// NewBlackboard constructs a fresh [core.Blackboard] using the
// configured [core.BlackboardFactory] extension (or the built-in
// in-memory implementation when none is registered). Public so
// orchestration helpers — most notably the workflow agent-level
// builders — can hand a child process a clean blackboard rather than
// inheriting the parent's accumulated state via
// [core.Blackboard.Spawn].
func (p *Platform) NewBlackboard() core.Blackboard { return p.resolveBlackboard(nil) }

// Agents returns a snapshot of registered agents.
func (p *Platform) Agents() []*core.Agent { return p.agents.list() }

// FindAgent does a name lookup.
func (p *Platform) FindAgent(name string) (*core.Agent, bool) { return p.agents.find(name) }

// GetProcess looks up a process by id.
func (p *Platform) GetProcess(id string) (*AgentProcess, bool) { return p.procs.get(id) }

// ActiveProcesses returns a snapshot of all currently registered
// processes.
func (p *Platform) ActiveProcesses() []*AgentProcess { return p.procs.list() }

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
