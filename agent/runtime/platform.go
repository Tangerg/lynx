package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/planner/goap"
)

// PlannerFactory is the abstraction that lets callers swap planners (A*
// vs. utility vs. mock) without touching the rest of the runtime. The
// default returns a fresh A* planner per call so planners aren't
// accidentally shared across processes that tune them independently.
type PlannerFactory func(t core.PlannerType) plan.Planner

// DefaultPlannerFactory returns the framework's default factory. Today
// it returns the A* GOAP planner regardless of t (utility planner not
// yet implemented).
func DefaultPlannerFactory() PlannerFactory {
	return func(_ core.PlannerType) plan.Planner {
		return goap.NewAStarPlanner()
	}
}

// Platform is the agent runtime's top-level container. It registers
// agents, builds processes, dispatches events, and exposes the resume
// API for HITL.
//
// Internal layout: agents and procs are named sub-struct fields rather
// than embedded promotions, so every access path is explicit at call
// sites. Platform exposes the public registry surface (Agents /
// FindAgent / GetProcess / ActiveProcesses) as thin wrappers that
// forward into the internal helpers (agents.list / find / etc.).
type Platform struct {
	agents agentRegistry   // Deploy-time registry of *core.Agent
	procs  processRegistry // Runtime registry of *AgentProcess

	plannerFactory PlannerFactory
	events         *event.Multicast
	idGen          IDGenerator

	services *core.ServiceProvider
	tools    core.ToolGroupResolver
}

// PlatformConfig is the construction-time configuration for [NewPlatform].
// All fields are optional — pass a zero PlatformConfig{} for a default
// platform with no services registered.
type PlatformConfig struct {
	// Services is the open-ended service registry handed to actions via
	// [core.ProcessContext.Services]. Pre-populate it with whatever LLM
	// clients, RAG engines, vector stores, or custom domain services
	// your actions need to look up. Nil means "start empty"; the
	// platform allocates a fresh provider.
	Services *core.ServiceProvider

	// Tools resolves agent-level [core.ToolGroupRequirement]s into
	// runnable tools at action-execution time. It's a separate field —
	// not a service registered under a magic key — because the runtime
	// itself reads from it during [core.ProcessContext.ResolveTools].
	Tools core.ToolGroupResolver

	// PlannerFactory overrides the default A* GOAP planner factory.
	PlannerFactory PlannerFactory

	// IDGenerator overrides the default UUID-v4 process ID generator —
	// useful for tests that want deterministic IDs.
	IDGenerator IDGenerator

	// Listeners are attached to the platform's event multicast at
	// construction time. [Platform.AddListener] adds more later.
	Listeners []event.Listener
}

// NewPlatform returns a fresh Platform from config. Zero-valued config fields
// fall back to defaults: A* planner factory, empty service registry,
// UUID-v4 id generator, no pre-attached listeners.
func NewPlatform(config PlatformConfig) *Platform {
	services := config.Services
	if services == nil {
		services = core.NewServiceProvider()
	}

	plannerFactory := config.PlannerFactory
	if plannerFactory == nil {
		plannerFactory = DefaultPlannerFactory()
	}

	idGen := config.IDGenerator
	if idGen == nil {
		idGen = NewUUIDIDGenerator()
	}

	p := &Platform{
		agents:         newAgentRegistry(),
		procs:          newProcessRegistry(),
		plannerFactory: plannerFactory,
		events:         event.NewMulticast(),
		idGen:          idGen,
		services:       services,
		tools:          config.Tools,
	}
	for _, listener := range config.Listeners {
		p.events.Add(listener)
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

// Services exposes the configured service provider — used by tests and
// the host application that wants to inspect what the platform sees.
func (p *Platform) Services() *core.ServiceProvider { return p.services }

// --- Registry surface (thin wrappers over agents / procs) ----------------

// Agents returns a snapshot of registered agents.
func (p *Platform) Agents() []*core.Agent { return p.agents.list() }

// FindAgent does a name lookup.
func (p *Platform) FindAgent(name string) (*core.Agent, bool) { return p.agents.find(name) }

// GetProcess looks up a process by id.
func (p *Platform) GetProcess(id string) (*AgentProcess, bool) { return p.procs.get(id) }

// ActiveProcesses returns a snapshot of all currently registered processes.
func (p *Platform) ActiveProcesses() []*AgentProcess { return p.procs.list() }

// RemoveProcess deletes a process from the registry. Mirrors embabel's
// AgentProcessRepository.delete: lets long-running services free
// terminal-state processes that the host has already drained
// (Status / Failure / Goal already read). Returns an error when the id
// is unknown so callers can detect typos.
func (p *Platform) RemoveProcess(id string) error {
	if !p.procs.unregister(id) {
		return processNotFoundError("remove process", id)
	}
	return nil
}

// PruneTerminalProcesses removes every registered process whose status
// satisfies [core.AgentProcessStatus.IsTerminal] and returns the
// removed ids. Convenient cleanup for long-lived hosts that don't want
// to track ids individually — call periodically or after a batch run.
func (p *Platform) PruneTerminalProcesses() []string {
	return p.procs.pruneWhere(func(proc *AgentProcess) bool {
		return proc.Status().IsTerminal()
	})
}

// Deploy registers an agent after validating it. Re-deploying with the
// same name replaces the previous registration — convenient when
// iterating during development.
//
// Validation runs in two layers:
//   - [core.ValidateAgent] checks structural invariants (name non-empty,
//     ≥1 action, ≥1 goal, unique action/goal names).
//   - For agents using [core.PlannerGOAP], each goal is then probed via
//     a one-step producer scan. Goals whose required True conditions no
//     action can establish are rejected with a clear error so
//     unreachable definitions (typo'd effect key, missing producing
//     action, etc.) fail at deploy time rather than at first tick.
func (p *Platform) Deploy(a *core.Agent) error {
	if err := core.ValidateAgent(a); err != nil {
		return fmt.Errorf("deploy agent: %w", err)
	}
	if err := checkGoalsReachable(a); err != nil {
		return fmt.Errorf("deploy agent %q: %w", a.Name, err)
	}

	p.agents.register(a)
	p.publish(event.AgentDeployedEvent{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: a.Name,
	})
	return nil
}

// checkGoalsReachable does a conservative one-step producer scan: for
// every condition each goal requires, we verify that either an action's
// effects can establish it OR an action's input binding looks like it
// (since input bindings come from process bindings + the dual-binding
// blackboard rules — they're considered reachable from "outside").
//
// This is intentionally weaker than running the full planner from empty
// state: that approach falsely rejects agents whose first action's
// precondition is "input binding present", because empty world state
// has no bindings. We accept the false-negative tradeoff (some
// genuinely-unreachable goals slip through) so legitimate input-driven
// agents can deploy.
func checkGoalsReachable(a *core.Agent) error {
	// Build the set of conditions any action can establish: union of
	// every action's Effects keys whose Determination is True, plus
	// every action's input bindings (those are externally-supplied).
	producible := map[string]struct{}{}
	for _, action := range a.Actions {
		if action == nil {
			return fmt.Errorf("action list contains a nil action")
		}
		meta := action.Metadata()
		for key, value := range meta.Effects {
			if value == core.True {
				producible[key] = struct{}{}
			}
		}
		for _, in := range meta.Inputs {
			producible[in.String()] = struct{}{}
		}
	}

	for _, goal := range a.Goals {
		for key, required := range goal.Preconditions() {
			if required != core.True {
				continue // we only flag missing producers for True-required conditions
			}
			if _, ok := producible[key]; !ok {
				return fmt.Errorf(
					"goal %q requires condition %q, but no action produces it",
					goal.Name, key,
				)
			}
		}
	}
	return nil
}

// Undeploy removes an agent. Returns an error when the name is unknown
// so callers don't silently miss typos.
func (p *Platform) Undeploy(name string) error {
	if err := p.agents.unregister(name); err != nil {
		return fmt.Errorf("undeploy agent %q: %w", name, err)
	}
	p.publish(event.AgentUndeployedEvent{
		BaseEvent: event.NewBaseEvent(""),
		AgentName: name,
	})
	return nil
}

// KillProcess terminates a running process. Returns an error when the
// id is unknown.
func (p *Platform) KillProcess(id string) error {
	proc, ok := p.GetProcess(id)
	if !ok {
		return processNotFoundError("kill process", id)
	}

	proc.state.setStatus(core.StatusKilled)
	p.publish(event.ProcessKilledEvent{
		BaseEvent: event.NewBaseEvent(id),
		Reason:    "kill requested",
	})
	return nil
}

// ResumeProcess delivers a response to a process parked on
// [AgentProcess.AwaitInput]. The awaitable's typed handler runs
// synchronously (typically mutating the blackboard) and returns the
// [core.ResponseImpact] decision. The process status stays
// [core.StatusWaiting] — call [Platform.ContinueProcess] /
// [Platform.ContinueProcessAsync] next to actually drive the run loop
// against the now-mutated blackboard.
//
// Splitting "deliver response" from "drive the loop" keeps ResumeProcess
// cheap, synchronous, and ctx-free, and lets the host control the
// continuation (sync vs background, fresh ctx vs the original).
//
// Returns an error when the id is unknown, the process isn't actually
// waiting, or the response value doesn't match the awaitable's expected
// type.
func (p *Platform) ResumeProcess(id string, response any) (core.ResponseImpact, error) {
	proc, ok := p.GetProcess(id)
	if !ok {
		return core.ResponseImpactUnchanged,
			processNotFoundError("resume process", id)
	}

	// deliverResponse atomically swaps the parked awaitable; that single
	// source of truth handles the "no awaitable pending" case so we
	// don't pre-check separately and race a concurrent resume.
	impact, err := proc.signals.deliverResponse(response)
	if err != nil {
		return core.ResponseImpactUnchanged, fmt.Errorf("resume process %q: %w", id, err)
	}
	return impact, nil
}

// ContinueProcess re-enters the run loop on an already-created process.
// Mirrors embabel's pattern of calling AgentProcess.run() repeatedly:
// after [Platform.ResumeProcess] delivers an awaitable response, or
// after a stuck-handler stages new blackboard state, ContinueProcess
// drives the OODA loop until the process exits Running again
// (terminal, waiting, or paused). Returns nil when the process exits
// normally; ctx-cancel and run errors propagate.
//
// Concurrent ContinueProcess calls on the same id are safe: the
// underlying makeRunning rejects when the process is already running,
// so only one call drives the loop.
func (p *Platform) ContinueProcess(ctx context.Context, id string) error {
	proc, ok := p.GetProcess(id)
	if !ok {
		return processNotFoundError("continue process", id)
	}
	return proc.run(normalizeContext(ctx))
}

// ContinueProcessAsync is the background variant of [ContinueProcess].
// Returns a buffered channel that receives the run's final error (nil
// on clean exit) so callers can fire-and-forget while still being able
// to wait on completion.
func (p *Platform) ContinueProcessAsync(ctx context.Context, id string) <-chan error {
	done := make(chan error, 1)

	proc, ok := p.GetProcess(id)
	if !ok {
		done <- processNotFoundError("continue process asynchronously", id)
		close(done)
		return done
	}

	go func() {
		done <- proc.run(normalizeContext(ctx))
		close(done)
	}()
	return done
}

// createProcess assembles an AgentProcess and its dependencies (blackboard,
// determiner, planner). The process is registered in the platform's map
// before being returned so concurrent ResumeProcess / KillProcess calls
// can find it.
func (p *Platform) createProcess(
	agentDef *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if agentDef == nil {
		return nil, errors.New("create process: agent definition is nil")
	}
	if err := core.ValidateAgent(agentDef); err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	options.ApplyDefaults()

	blackboard := options.Blackboard
	if blackboard == nil {
		blackboard = newInMemoryBlackboard()
	}
	bindBlackboardSeed(blackboard, bindings)

	plannerFactory := p.plannerFactory
	if plannerFactory == nil {
		plannerFactory = DefaultPlannerFactory()
	}

	planner := plannerFactory(options.PlannerType)
	if planner == nil {
		return nil, fmt.Errorf("create process for agent %q: planner factory returned nil for %s planner", agentDef.Name, options.PlannerType)
	}
	system := plan.FromAgent(agentDef)
	id := p.idGen.Next()

	proc := newAgentProcess(id, agentDef, &options, blackboard, nil, planner, system, p)
	proc.determiner = newBlackboardDeterminer(system, blackboard, proc)

	p.procs.register(proc)

	p.publish(event.ProcessCreatedEvent{
		BaseEvent: event.NewBaseEvent(id),
		Bindings:  bindings,
	})
	return proc, nil
}

// bindBlackboardSeed applies the caller's initial bindings. The DefaultBinding
// key uses Bind() so the dual-binding behavior kicks in; other keys go
// through Set so their explicit name wins.
func bindBlackboardSeed(blackboard core.Blackboard, bindings map[string]any) {
	for key, value := range bindings {
		if key == core.DefaultBindingName {
			blackboard.Bind(value)
			continue
		}
		blackboard.Set(key, value)
	}
}

// RunAgent runs the named agent synchronously and returns the resulting
// process (whether completed or terminal-failed). Pass a zero
// [core.ProcessOptions]{} for defaults.
func (p *Platform) RunAgent(
	ctx context.Context,
	agentDef *core.Agent,
	bindings map[string]any,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	proc, err := p.createProcess(agentDef, bindings, options)
	if err != nil {
		return nil, err
	}

	if err := proc.run(normalizeContext(ctx)); err != nil {
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
	options core.ProcessOptions,
) (*AgentProcess, <-chan error) {
	done := make(chan error, 1)

	proc, err := p.createProcess(agentDef, bindings, options)
	if err != nil {
		done <- err
		close(done)
		return nil, done
	}

	go func() {
		done <- proc.run(normalizeContext(ctx))
		close(done)
	}()
	return proc, done
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// CreateChildProcess spawns a sub-agent process whose blackboard inherits
// the parent's. Used by composite agents that delegate sub-tasks.
func (p *Platform) CreateChildProcess(
	agentDef *core.Agent,
	parent *AgentProcess,
	options core.ProcessOptions,
) (*AgentProcess, error) {
	if parent == nil {
		return nil, errors.New("create child process: parent process is nil")
	}

	// Inherit the parent's blackboard unless the caller supplied one.
	if options.Blackboard == nil {
		options.Blackboard = parent.Blackboard().Spawn()
	}

	child, err := p.createProcess(agentDef, nil, options)
	if err != nil {
		return nil, err
	}
	child.parentID = parent.id

	// Register the child on the parent so Usage() can recursively roll
	// up cost / tokens / actions across the whole delegation tree.
	parent.budget.addChild(child)

	return child, nil
}

func processNotFoundError(operation, id string) error {
	return fmt.Errorf("%s: process %q not found", operation, id)
}
