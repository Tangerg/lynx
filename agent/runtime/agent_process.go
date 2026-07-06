package runtime

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// AgentProcess is the runtime's mutable per-execution state. It implements
// core.Process (read surface) plus mutation methods (TerminateAgent,
// AwaitInput, ...) that only the runtime should call.
//
// Internal layout — three concerns kept as named sub-struct fields so
// related fields & methods cluster together while the access path stays
// explicit at every call site:
//
//   - state    mu-protected status / goal / history / failure /
//     exclusions. Owns the main mutex; budget shares it via
//     a pointer.
//   - budget   subtree cost / token / action aggregation; lock pointer
//     points at state.mu.
//   - signals  channel + atomic-based signaling primitives
//     (terminate / awaitable slot / toolCallCancel) — no
//     shared lock, all built on lock-free primitives.
//
// The remaining top-level fields are construction-time wiring (id /
// agent / options / blackboard / determiner / planner / system /
// platform) — immutable after newAgentProcess returns.
type AgentProcess struct {
	id        string
	parentID  string
	depth     int // delegation depth: 0 at top level, parent+1 for a child
	agent     *core.Agent
	options   *core.ProcessOptions
	startedAt time.Time

	state   processState
	budget  processBudget
	signals processSignals

	blackboard core.Blackboard
	determiner *blackboardDeterminer
	planner    planning.Planner
	system     *planning.System
	platform   *Platform

	// processEvents is the per-process multicast populated from
	// EventListener extensions on ProcessOptions.Extensions. Wired by
	// wireRuntimeDeps on every construction path (createProcess +
	// RestoreProcess); publishEvent still nil-guards it for safety.
	processEvents *event.Multicast
}

// ActionInvocation is one row of the per-process history.
type ActionInvocation struct {
	ActionName string
	Timestamp  time.Time
	Duration   time.Duration
	Status     core.ActionStatus
	Attempts   int
}

// newAgentProcess assembles a process from its inputs. Internal — users
// invoke Platform.RunAgent which assembles every dependency. The
// determiner and processEvents are populated by the caller after
// construction because both need the *AgentProcess pointer (the
// determiner wires it as the [core.Process] for user conditions; the
// multicast subscribes to per-process EventListener extensions).
func newAgentProcess(
	id string,
	agentDef *core.Agent,
	options *core.ProcessOptions,
	blackboard core.Blackboard,
	planner planning.Planner,
	system *planning.System,
	platform *Platform,
) *AgentProcess {
	p := &AgentProcess{
		id:         id,
		agent:      agentDef,
		options:    options,
		startedAt:  core.Now(),
		state:      newProcessState(),
		signals:    newProcessSignals(),
		blackboard: blackboard,
		planner:    planner,
		system:     system,
		platform:   platform,
	}
	p.budget.lock = &p.state.mu // budget shares state's mutex
	return p
}

// wireRuntimeDeps finishes the parts of construction that need the
// *AgentProcess pointer itself: the determiner (which wires the process
// as the [core.Process] user-defined conditions evaluate against) and
// the per-process event multicast (subscribing process-scope
// EventListener extensions). Split out of newAgentProcess because both
// fields close over the assembled pointer, and shared by every path
// that builds a process — createProcess for fresh runs, RestoreProcess
// for snapshots re-entering the tick loop. A restored process that
// skips this panics on its first observe (nil determiner).
func (p *AgentProcess) wireRuntimeDeps(extensions []core.Extension) {
	p.determiner = newBlackboardDeterminer(p.system, p.blackboard, p)
	p.processEvents = event.NewMulticast()
	addEventListenerExtensions(p.processEvents, extensions)
}

// --- core.Process read surface --------------------------------------------

func (p *AgentProcess) ID() string                    { return p.id }
func (p *AgentProcess) ParentID() string              { return p.parentID }
func (p *AgentProcess) StartedAt() time.Time          { return p.startedAt }
func (p *AgentProcess) Blackboard() core.Blackboard   { return p.blackboard }
func (p *AgentProcess) Options() *core.ProcessOptions { return p.options }

// conversationID returns the chat history conversation id for this
// process; the derivation rule (session id when under a session, else
// process id) lives in [core.ConversationID].
func (p *AgentProcess) conversationID() string {
	if p == nil {
		return ""
	}
	return core.ConversationID(p.options, p.id)
}

// userID returns the principal this process runs as, inherited by child
// sessions so audit trails span the delegation subtree.
func (p *AgentProcess) userID() string {
	if p == nil {
		return ""
	}
	if p.options != nil && p.options.Session != nil {
		return p.options.Session.UserID
	}
	return ""
}

// Status / Goal / LastWorldState / Failure / History delegate to the
// state sub-struct, which owns the lock.
func (p *AgentProcess) Status() core.AgentProcessStatus { return p.state.getStatus() }
func (p *AgentProcess) Goal() *core.Goal                { return p.state.getGoal() }
func (p *AgentProcess) LastWorldState() core.WorldState { return p.state.getLastWorld() }
func (p *AgentProcess) Failure() error                  { return p.state.getFailure() }
func (p *AgentProcess) History() []ActionInvocation     { return p.state.getHistory() }
