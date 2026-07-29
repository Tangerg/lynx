package runtime

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// Process is the runtime's mutable per-execution state. It implements the
// read, control, and usage capabilities that runtime composition grants
// separately to consumers.
//
// Concerns are grouped into named sub-structs so each carries its own
// synchronization: nested holds a mutex of its own because sibling AgentTools
// update it concurrently, while signals is lock-free throughout. state owns
// checkpointing so a suspension transition and a portable capture serialize
// against each other without any lock being held across an extension callback.
// Every remaining field is assembled before the process is published to the
// registry and never written again.
type Process struct {
	id                 string
	parentID           string
	spawnCallID        string
	depth              int // delegation depth: 0 at top level, parent+1 for a child
	deployment         *Deployment
	deploymentRetained bool
	options            *processOptions
	startedAt          time.Time

	state   processState
	budget  processBudget
	signals processSignals
	nested  nestedChildState

	blackboard   core.Blackboard
	dependencies *core.Dependencies
	stateReader  *worldStateReader
	planner      planning.Planner
	domain       *planning.Domain
	engine       *Engine

	// processEvents is populated from the EventListener extensions on
	// ProcessOptions.Extensions. A process built without [Process.wireRuntimeDeps]
	// leaves it nil, which is why publishing nil-guards it.
	processEvents *event.Multicast
}

func (p *Process) releaseDeployment() {
	if !p.deploymentRetained || p.engine == nil {
		return
	}
	p.engine.catalog.release(p.deployment)
	p.deploymentRetained = false
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
		budget:       newProcessBudget(options.budget),
		signals:      newProcessSignals(),
		blackboard:   blackboard,
		dependencies: dependencies,
		planner:      planner,
		domain:       domain,
		engine:       engine,
	}
	return p
}

func (p *Process) agent() *core.Agent {
	if p.deployment == nil {
		return nil
	}
	return p.deployment.agent
}

// wireRuntimeDeps assigns the fields that close over the assembled *Process,
// which is why they cannot be set in the constructor. Every path that builds a
// process must call it: one that skips it panics on its first observe against a
// nil state reader.
func (p *Process) wireRuntimeDeps(extensions []extensionEntry) {
	p.stateReader = newWorldStateReader(p.domain, p.blackboard, p)
	p.processEvents = event.NewMulticast()
	addEventListenerExtensions(p.processEvents, extensions)
}

// --- core.ProcessView read surface ----------------------------------------

func (p *Process) ID() string                        { return p.id }
func (p *Process) ParentID() string                  { return p.parentID }
func (p *Process) SpawnCallID() string               { return p.spawnCallID }
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

func (p *Process) Status() core.ProcessStatus { return p.state.status() }
func (p *Process) Goal() core.GoalDescriptor  { return p.state.goal().Descriptor() }
func (p *Process) WorldState() core.WorldState {
	return p.state.worldState()
}
func (p *Process) Failure() error { return p.state.failure() }
