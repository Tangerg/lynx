package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
)

// Platform is the agent runtime's top-level container. It registers
// agents, builds processes, dispatches events, and exposes the resume
// API for HITL.
//
// All pluggable behaviour (event listeners, action interceptors, tool
// decorators, agent validators, goal approvers, tool-group resolvers,
// id generators, planner factories, blackboard factories) flows
// through one mechanism: registered [core.Extension]s. Platform-scoped
// extensions are passed via [PlatformConfig.Extensions] at construction;
// per-process extensions live on [core.ProcessOptions.Extensions] and
// merge with platform extensions when the runtime walks dispatch chains
// for a specific [AgentProcess].
type Platform struct {
	agents agentRegistry   // Deploy-time registry of *core.Agent
	procs  processRegistry // Runtime registry of *AgentProcess

	extensions extensionRegistry // platform-scoped extensions

	events   *event.Multicast       // populated from EventListener extensions
	services *core.ServiceProvider  // open registry exposed via Platform.Services()
}

// PlatformConfig is the construction-time configuration for [NewPlatform].
// Pass a zero PlatformConfig{} to get a platform with framework defaults
// — built-in UUID id generator, GOAP A* planner factory, in-memory
// blackboard factory, no listeners, no tool resolvers, empty service
// registry. Add extensions to override / extend any of those.
type PlatformConfig struct {
	// Extensions are the platform-scoped plug-ins. Each value must
	// implement [core.Extension] and may additionally implement any
	// subset of the capability interfaces (EventListener,
	// ActionInterceptor, ToolDecorator, AgentValidator, GoalApprover,
	// ToolGroupResolver, IDGenerator, PlannerFactory, BlackboardFactory)
	// — the runtime detects each capability via type assertion at
	// dispatch time.
	//
	// Within Extensions, [core.Extension.Name] must be unique; an empty
	// or duplicate Name causes [NewPlatform] to panic so boot-time
	// configuration errors fail fast.
	Extensions []core.Extension
}

// NewPlatform returns a fresh Platform from config. Panics on invalid
// extension registration (nil extension, empty Name, duplicate Name).
func NewPlatform(config PlatformConfig) *Platform {
	p := &Platform{
		agents:     newAgentRegistry(),
		procs:      newProcessRegistry(),
		extensions: newExtensionRegistry(),
		events:     event.NewMulticast(),
		services:   core.NewServiceProvider(),
	}
	for _, ext := range config.Extensions {
		p.extensions.register("PlatformConfig.Extensions", ext)
	}
	addEventListenerExtensions(p.events, p.extensions.list)
	return p
}

// publish is the runtime's event entry point. Used by AgentProcess and
// executeAction.
func (p *Platform) publish(e event.Event) {
	if e == nil {
		return
	}
	p.events.OnEvent(e)
}

// Services exposes the platform-internal service registry — used by
// the host application to register LLM clients, RAG engines, vector
// stores, or other domain services that actions look up by key.
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
// Validation runs in three layers:
//
//   - [core.ValidateAgent] checks structural invariants (name non-empty,
//     ≥1 action, ≥1 goal, unique action/goal names).
//   - For agents using [core.PlannerGOAP], each goal is then probed via
//     a one-step producer scan. Goals whose required True conditions no
//     action can establish are rejected with a clear error so
//     unreachable definitions fail at deploy time rather than at first tick.
//   - Every registered [core.AgentValidator] extension runs in
//     registration order — the first to return an error rejects the
//     deployment, with the validator's Name attached for attribution.
func (p *Platform) Deploy(a *core.Agent) error {
	if err := core.ValidateAgent(a); err != nil {
		return fmt.Errorf("deploy agent: %w", err)
	}
	if err := checkGoalsReachable(a); err != nil {
		return fmt.Errorf("deploy agent %q: %w", a.Name, err)
	}
	if err := runAgentValidators(collectExtensions[core.AgentValidator](p.extensions.list), a); err != nil {
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

// idGenerator returns the most-recently-registered IDGenerator
// extension, falling back to a UUID-v4 generator when none is registered.
func (p *Platform) idGenerator() core.IDGenerator {
	if g := lastExtension[core.IDGenerator](p.extensions.list); g != nil {
		return g
	}
	return defaultIDGenerator
}

// plannerFactory mirrors idGenerator for PlannerFactory.
func (p *Platform) plannerFactory() PlannerFactory {
	if f := lastExtension[PlannerFactory](p.extensions.list); f != nil {
		return f
	}
	return defaultPlannerFactoryInstance
}

// blackboardFactory returns the most-recently-registered BlackboardFactory
// extension or nil — callers fall back to the in-memory blackboard.
func (p *Platform) blackboardFactory() core.BlackboardFactory {
	return lastExtension[core.BlackboardFactory](p.extensions.list)
}

// Built-in fallbacks for last-wins singletons. Lazily initialised once.
var (
	defaultIDGenerator            = core.NewUUIDIDGenerator("")
	defaultPlannerFactoryInstance = DefaultPlannerFactory()
)

// createProcess assembles an AgentProcess and its dependencies (blackboard,
// determiner, planner). The process is registered in the platform's map
// before being returned so concurrent ResumeProcess / KillProcess calls
// can find it. Process-scope extensions on options are validated for
// dedup / nil / empty-Name before any work happens — invalid extensions
// turn the call into an error rather than letting the process start
// and panic later.
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
	if err := validateProcessExtensions(options.Extensions); err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	options.ApplyDefaults()

	blackboard := p.resolveBlackboard(options.Blackboard)
	bindBlackboardSeed(blackboard, bindings)

	planner := p.plannerFactory().NewPlanner(options.PlannerType)
	if planner == nil {
		return nil, fmt.Errorf("create process for agent %q: planner factory returned nil for %s planner", agentDef.Name, options.PlannerType)
	}

	system := plan.FromAgent(agentDef)
	id := p.idGenerator().Next()
	proc := newAgentProcess(id, agentDef, &options, blackboard, planner, system, p)

	// Both fields below need the *AgentProcess pointer — set them after
	// construction. determiner uses it as the [core.Process] handed to
	// user-defined conditions; processEvents subscribes process-scope
	// EventListener extensions so they only see this process's events.
	proc.determiner = newBlackboardDeterminer(system, blackboard, proc)
	proc.processEvents = event.NewMulticast()
	addEventListenerExtensions(proc.processEvents, options.Extensions)

	p.procs.register(proc)

	p.publish(event.ProcessCreatedEvent{
		BaseEvent: event.NewBaseEvent(id),
		Bindings:  bindings,
	})
	return proc, nil
}

// resolveBlackboard picks the [core.Blackboard] for a fresh process —
// per-call value wins; otherwise the registered [core.BlackboardFactory]
// extension; otherwise the built-in in-memory implementation.
func (p *Platform) resolveBlackboard(supplied core.Blackboard) core.Blackboard {
	if supplied != nil {
		return supplied
	}
	if factory := p.blackboardFactory(); factory != nil {
		if bb := factory.NewBlackboard(); bb != nil {
			return bb
		}
	}
	return newInMemoryBlackboard()
}

// validateProcessExtensions enforces the per-process invariants — nil
// entries are rejected, empty Names are rejected, duplicate Names within
// the slice are rejected. Process-scope Names are allowed to collide with
// platform-scope Names (that's the explicit override mechanism).
func validateProcessExtensions(extensions []core.Extension) error {
	if len(extensions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(extensions))
	for i, ext := range extensions {
		if ext == nil {
			return fmt.Errorf("ProcessOptions.Extensions[%d] is nil", i)
		}
		name := ext.Name()
		if name == "" {
			return fmt.Errorf("ProcessOptions.Extensions[%d] (%T) returned empty Name()", i, ext)
		}
		if _, dup := seen[name]; dup {
			return fmt.Errorf("ProcessOptions.Extensions: duplicate name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
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
