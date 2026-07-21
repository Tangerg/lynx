package runtime

import (
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// Process is the runtime's mutable per-execution state. It implements the
// read, control, and usage capabilities that runtime composition grants
// separately to consumers.
//
// Internal layout — four concerns kept as named sub-struct fields so
// related fields & methods cluster together while the access path stays
// explicit at every call site:
//
//   - state    mu-protected status / goal / history / failure /
//     exclusions. Owns the main mutex; budget shares it via
//     a pointer.
//   - budget   subtree cost / token / action aggregation; lock pointer
//     points at state.mu.
//   - signals  channel + atomic-based signaling primitives
//     (terminate / toolCallCancel) — no
//     shared lock, all built on lock-free primitives.
//   - nested   nested-child ownership, suspension staging, and deferred
//     cleanup; owns a separate mutex because sibling AgentTools may update it
//     concurrently.
//
// checkpointMu serializes suspension transitions and durable tree capture.
// The other top-level fields are construction-time wiring (id /
// deployment / options / blackboard / state reader / planner / domain /
// engine) — immutable after newProcess returns.
type Process struct {
	id         string
	parentID   string
	depth      int // delegation depth: 0 at top level, parent+1 for a child
	deployment *Deployment
	options    *processOptions
	startedAt  time.Time

	state        processState
	budget       processBudget
	signals      processSignals
	nested       nestedChildState
	checkpointMu sync.RWMutex

	blackboard   core.Blackboard
	dependencies *core.Dependencies
	stateReader  *worldStateReader
	planner      planning.Planner
	domain       *planning.Domain
	engine       *Engine

	// processEvents is the per-process multicast populated from
	// EventListener extensions on ProcessOptions.Extensions. Wired by
	// wireRuntimeDeps on every construction path (createProcess +
	// Restore); publishEvent still nil-guards it for safety.
	processEvents *event.Multicast
}

// ActionRun is one row of the per-process history.
type ActionRun struct {
	ActionName string
	StartedAt  time.Time
	Duration   time.Duration
	Status     core.ActionStatus
}

// newProcess assembles a process from its inputs. Internal — users
// invoke Engine.Run which assembles every dependency. The
// state reader and processEvents are populated by the caller after
// construction because both need the *Process pointer (the
// state reader wires it as the [core.ProcessView] for user conditions; the
// multicast subscribes to per-process EventListener extensions).
func newProcess(
	id string,
	deployment *Deployment,
	options *processOptions,
	blackboard core.Blackboard,
	dependencies *core.Dependencies,
	planner planning.Planner,
	domain *planning.Domain,
	engine *Engine,
) *Process {
	p := &Process{
		id:           id,
		deployment:   deployment,
		options:      options,
		startedAt:    time.Now(),
		state:        newProcessState(),
		signals:      newProcessSignals(),
		blackboard:   blackboard,
		dependencies: dependencies,
		planner:      planner,
		domain:       domain,
		engine:       engine,
	}
	p.budget.lock = &p.state.mu // budget shares state's mutex
	return p
}

func (p *Process) agent() *core.Agent {
	if p == nil || p.deployment == nil {
		return nil
	}
	return p.deployment.agent
}

// wireRuntimeDeps finishes the parts of construction that need the
// *Process pointer itself: the state reader (which wires the process
// as the [core.ProcessView] user-defined conditions evaluate against) and
// the per-process event multicast (subscribing process-scope
// EventListener extensions). Split out of newProcess because both
// fields close over the assembled pointer, and shared by every path
// that builds a process — createProcess for fresh runs, Restore
// for snapshots re-entering the tick loop. A restored process that
// skips this panics on its first observe (nil state reader).
func (p *Process) wireRuntimeDeps(extensions []core.Extension) {
	p.stateReader = newWorldStateReader(p.domain, p.blackboard, p)
	p.processEvents = event.NewMulticast()
	addEventListenerExtensions(p.processEvents, extensions)
}

// --- core.ProcessView read surface ----------------------------------------

func (p *Process) ID() string                        { return p.id }
func (p *Process) ParentID() string                  { return p.parentID }
func (p *Process) StartedAt() time.Time              { return p.startedAt }
func (p *Process) Blackboard() core.BlackboardReader { return p.blackboard }

// DeploymentRef returns the exact immutable definition identity bound when
// this process was created. Redeploying the same agent name cannot change it.
func (p *Process) Deployment() core.DeploymentRef {
	if p == nil || p.deployment == nil {
		return core.DeploymentRef{}
	}
	return p.deployment.Ref()
}

// conversationID returns the chat history partition for this process: the
// session id for a multi-turn run, otherwise the process id.
func (p *Process) conversationID() string {
	if p == nil {
		return ""
	}
	if p.options != nil && p.options.session != nil && p.options.session.ID != "" {
		return p.options.session.ID
	}
	return p.id
}

// userID returns the principal this process runs as, inherited by child
// sessions so audit trails span the delegation subtree.
func (p *Process) userID() string {
	if p == nil {
		return ""
	}
	if p.options != nil && p.options.session != nil {
		return p.options.session.UserID
	}
	return ""
}

func (p *Process) Status() core.ProcessStatus  { return p.state.status() }
func (p *Process) Goal() *core.Goal            { return p.state.goal() }
func (p *Process) WorldState() core.WorldState { return p.state.worldState() }
func (p *Process) Failure() error              { return p.state.failure() }
func (p *Process) History() []ActionRun        { return p.state.historySnapshot() }
