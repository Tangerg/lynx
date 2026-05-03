package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/planner/goap"
)

// PlannerFactory is the abstraction that lets callers swap planners (A* vs.
// utility vs. mock) without touching the rest of the runtime. The default
// returns a fresh A* planner per call so planners aren't accidentally
// shared across processes that tune them independently.
type PlannerFactory func(t core.PlannerType) plan.Planner

// DefaultPlannerFactory returns the framework's default factory. Today it
// returns the A* GOAP planner regardless of t (utility planner not yet
// implemented).
func DefaultPlannerFactory() PlannerFactory {
	return func(_ core.PlannerType) plan.Planner {
		return goap.NewAStarPlanner()
	}
}

// Platform is the agent runtime's top-level container. It registers agents,
// builds processes, dispatches events, and exposes the resume API for HITL.
type Platform struct {
	mu     sync.RWMutex
	agents map[string]*core.Agent
	procs  map[string]*AgentProcess

	plannerFactory PlannerFactory
	events         *event.Multicast
	idGen          IDGenerator

	services *core.ServiceProvider
}

// PlatformConfig is the construction-time configuration for [NewPlatform].
// All fields are optional — pass a zero PlatformConfig{} for the default
// platform. Per-agent service slots (Chat, RAG, VectorStore, Tools) are
// merged into a fresh [core.ServiceProvider] unless a pre-built
// ServiceProvider is supplied.
type PlatformConfig struct {
	// Chat is the default LLM client. The platform doesn't refuse
	// construction without one because integration tests often run
	// agents that never call the LLM.
	Chat core.ChatClient

	// RAG is the optional retrieval-augmented-generation client.
	RAG core.RAGClient

	// VectorStore is the optional embedding store used by RAG.
	VectorStore core.VectorStore

	// Tools resolves agent-level [ToolGroupRequirement]s into runnable
	// tools.
	Tools core.ToolGroupResolver

	// ServiceProvider, when non-nil, replaces the per-field Chat/RAG/
	// VectorStore/Tools entries entirely. Use it to share one service
	// graph across multiple platforms.
	ServiceProvider *core.ServiceProvider

	// PlannerFactory overrides the default A* GOAP planner factory.
	PlannerFactory PlannerFactory

	// IDGenerator overrides the default UUID-v4 process ID generator —
	// useful for tests that want deterministic IDs.
	IDGenerator IDGenerator

	// Listeners are attached to the platform's event multicast at
	// construction time. [Platform.AddListener] adds more later.
	Listeners []event.Listener
}

// NewPlatform returns a fresh Platform from cfg. Zero-valued cfg fields
// fall back to defaults: A* planner factory, fresh service provider,
// UUID-v4 id generator, no pre-attached listeners.
func NewPlatform(cfg PlatformConfig) *Platform {
	services := cfg.ServiceProvider
	if services == nil {
		services = &core.ServiceProvider{
			Chat:        cfg.Chat,
			RAG:         cfg.RAG,
			VectorStore: cfg.VectorStore,
			Tools:       cfg.Tools,
		}
	}

	plannerFactory := cfg.PlannerFactory
	if plannerFactory == nil {
		plannerFactory = DefaultPlannerFactory()
	}

	idGen := cfg.IDGenerator
	if idGen == nil {
		idGen = NewUUIDIDGenerator()
	}

	p := &Platform{
		agents:         map[string]*core.Agent{},
		procs:          map[string]*AgentProcess{},
		plannerFactory: plannerFactory,
		events:         event.NewMulticast(),
		idGen:          idGen,
		services:       services,
	}
	for _, l := range cfg.Listeners {
		p.events.Add(l)
	}
	return p
}

// AddListener registers an event listener. Listeners are delivered in
// registration order; non-blocking by convention.
func (p *Platform) AddListener(l event.Listener) { p.events.Add(l) }

// RemoveListener detaches a listener.
func (p *Platform) RemoveListener(l event.Listener) { p.events.Remove(l) }

// publish is the runtime's event entry point. Used by AgentProcess and
// executeAction.
func (p *Platform) publish(e event.Event) {
	if e == nil {
		return
	}
	p.events.OnEvent(e)
}

// Services exposes the configured service provider — used by tests and the
// host application that wants to inspect what the platform sees.
func (p *Platform) Services() *core.ServiceProvider { return p.services }

// Deploy registers an agent. Re-deploying with the same name replaces the
// previous registration — convenient when iterating during development.
func (p *Platform) Deploy(a *core.Agent) error {
	if a == nil {
		return errors.New("Deploy: agent is nil")
	}
	if a.Name == "" {
		return errors.New("Deploy: agent must have a non-empty Name")
	}

	p.mu.Lock()
	p.agents[a.Name] = a
	p.mu.Unlock()

	p.publish(event.AgentDeployedEvent{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: a.Name,
	})
	return nil
}

// Undeploy removes an agent. Returns an error when the name is unknown so
// callers don't silently miss typos.
func (p *Platform) Undeploy(name string) error {
	p.mu.Lock()
	if _, ok := p.agents[name]; !ok {
		p.mu.Unlock()
		return fmt.Errorf("Undeploy: agent %q is not deployed", name)
	}
	delete(p.agents, name)
	p.mu.Unlock()

	p.publish(event.AgentUndeployedEvent{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: name,
	})
	return nil
}

// Agents returns a snapshot of registered agents.
func (p *Platform) Agents() []*core.Agent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := make([]*core.Agent, 0, len(p.agents))
	for _, a := range p.agents {
		out = append(out, a)
	}
	return out
}

// FindAgent does a name lookup.
func (p *Platform) FindAgent(name string) (*core.Agent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	a, ok := p.agents[name]
	return a, ok
}

// GetProcess looks up a process by id (used by the HITL resume API and by
// debug tools).
func (p *Platform) GetProcess(id string) (*AgentProcess, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	proc, ok := p.procs[id]
	return proc, ok
}

// ActiveProcesses returns a snapshot of all currently registered processes.
func (p *Platform) ActiveProcesses() []*AgentProcess {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return slices.Collect(func(yield func(*AgentProcess) bool) {
		for _, proc := range p.procs {
			if !yield(proc) {
				return
			}
		}
	})
}

// KillProcess terminates a running process. Returns an error when the id is
// unknown.
func (p *Platform) KillProcess(id string) error {
	proc, ok := p.GetProcess(id)
	if !ok {
		return fmt.Errorf("KillProcess: process id %q not found", id)
	}

	proc.setStatus(core.StatusKilled)
	p.publish(event.ProcessKilledEvent{
		BaseEvent: event.NewBaseEvent(id),
		Reason:    "kill requested",
	})
	return nil
}

// ResumeProcess delivers a response to a paused process. Returns an error
// when the id is unknown or the process isn't waiting.
func (p *Platform) ResumeProcess(id string, response any) error {
	proc, ok := p.GetProcess(id)
	if !ok {
		return fmt.Errorf("ResumeProcess: process id %q not found", id)
	}
	if proc.peekAwaitable() == nil {
		return fmt.Errorf("ResumeProcess: process %q is not in a waiting state", id)
	}

	if !proc.deliverResponse(response) {
		return fmt.Errorf("ResumeProcess: failed to deliver response to process %q", id)
	}
	proc.setStatus(core.StatusRunning)
	return nil
}

// createProcess assembles an AgentProcess and its dependencies (blackboard,
// determiner, planner). The process is registered in the platform's map
// before being returned so concurrent ResumeProcess / KillProcess calls
// can find it.
func (p *Platform) createProcess(
	agentDef *core.Agent,
	bindings map[string]any,
	opts core.ProcessOptions,
) (*AgentProcess, error) {
	if agentDef == nil {
		return nil, errors.New("createProcess: agent definition is nil")
	}
	opts.ApplyDefaults()

	bb := opts.Blackboard
	if bb == nil {
		bb = newInMemoryBlackboard()
	}
	bindBlackboardSeed(bb, bindings)

	planner := p.plannerFactory(opts.PlannerType)
	system := plan.FromAgent(agentDef)
	id := p.idGen.Next()

	proc := newAgentProcess(id, agentDef, &opts, bb, nil, planner, p)
	proc.determiner = newBlackboardDeterminer(system, bb, proc)

	p.mu.Lock()
	p.procs[id] = proc
	p.mu.Unlock()

	p.publish(event.ProcessCreatedEvent{
		BaseEvent: event.NewBaseEvent(id),
		Bindings:  bindings,
	})
	return proc, nil
}

// bindBlackboardSeed applies the caller's initial bindings. The DefaultBinding
// key uses Bind() so the dual-binding behavior kicks in; other keys go
// through Set so their explicit name wins.
func bindBlackboardSeed(bb core.Blackboard, bindings map[string]any) {
	for key, value := range bindings {
		if key == core.DefaultBindingName {
			bb.Bind(value)
			continue
		}
		bb.Set(key, value)
	}
}

// RunAgent runs the named agent synchronously and returns the resulting
// process (whether completed or terminal-failed). Pass a zero
// [core.ProcessOptions]{} for defaults.
func (p *Platform) RunAgent(
	ctx context.Context,
	agentDef *core.Agent,
	bindings map[string]any,
	opts core.ProcessOptions,
) (*AgentProcess, error) {
	proc, err := p.createProcess(agentDef, bindings, opts)
	if err != nil {
		return nil, err
	}

	if err := proc.run(ctx); err != nil {
		return proc, err
	}
	return proc, nil
}

// StartAgent runs the agent in the background, returning the process and a
// channel that delivers the final error (or nil on success).
func (p *Platform) StartAgent(
	ctx context.Context,
	agentDef *core.Agent,
	bindings map[string]any,
	opts core.ProcessOptions,
) (*AgentProcess, <-chan error) {
	done := make(chan error, 1)

	proc, err := p.createProcess(agentDef, bindings, opts)
	if err != nil {
		done <- err
		close(done)
		return nil, done
	}

	go func() {
		done <- proc.run(ctx)
		close(done)
	}()
	return proc, done
}

// CreateChildProcess spawns a sub-agent process whose blackboard inherits
// the parent's. Used by composite agents that delegate sub-tasks.
func (p *Platform) CreateChildProcess(
	agentDef *core.Agent,
	parent *AgentProcess,
	opts core.ProcessOptions,
) (*AgentProcess, error) {
	if parent == nil {
		return nil, errors.New("CreateChildProcess: parent process is nil")
	}

	// Inherit the parent's blackboard unless the caller supplied one.
	if opts.Blackboard == nil {
		opts.Blackboard = parent.Blackboard().Spawn()
	}

	child, err := p.createProcess(agentDef, nil, opts)
	if err != nil {
		return nil, err
	}
	child.parentID = parent.id
	return child, nil
}
