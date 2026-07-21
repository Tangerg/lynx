package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// ErrDeploymentConflict reports that Deploy was asked to change an existing
// active route. Call Replace explicitly when the new definition is intended.
var ErrDeploymentConflict = errors.New("deployment conflict")

// ErrDeploymentNotFound reports that an active or exact deployment lookup
// could not be satisfied.
var ErrDeploymentNotFound = errors.New("deployment not found")

// ErrDurableIdentityRequired reports that a persistent Engine cannot prove
// executable identity because both Agent.Version and Config.BuildID
// are empty.
var ErrDurableIdentityRequired = errors.New("durable deployment requires Agent.Version or Config.BuildID")

// DeploymentConflictError describes the active and candidate identities that
// collided. It unwraps to ErrDeploymentConflict for errors.Is and retains both
// refs for errors.As diagnostics.
type DeploymentConflictError struct {
	Active    core.DeploymentRef
	Candidate core.DeploymentRef
}

func (e *DeploymentConflictError) Error() string {
	if e == nil {
		return ErrDeploymentConflict.Error()
	}
	return fmt.Sprintf("%s: active %s, candidate %s; use Engine.Replace to change the active route", ErrDeploymentConflict, e.Active, e.Candidate)
}

func (e *DeploymentConflictError) Unwrap() error { return ErrDeploymentConflict }

// Deploy registers an agent after a multi-layer validation that reports
// every problem at once rather than stopping at the first:
//
//  1. Runtime freezes caller-owned SPI metadata into the execution snapshot.
//  2. [core.Agent.Validate] checks that frozen snapshot's structural invariants.
//  3. Every [core.AgentValidator] extension runs against that same snapshot;
//     each error is collected with the validator's Name attributed.
//
// All collected problems are joined into a single error so a misconfigured
// agent surfaces its full problem list in one deploy attempt.
//
// Deploy installs the first active deployment for an agent name. Repeating the
// exact same DeploymentRef is idempotent; a different definition returns
// ErrDeploymentConflict. Use Replace for an intentional route change.
// Deployment itself is local and non-blocking; ctx is carried to listeners and
// tracing rather than used as a cancellation boundary.
func (e *Engine) Deploy(ctx context.Context, agent *core.Agent) (*Deployment, error) {
	return e.deploy(normalizeContext(ctx), agent, false)
}

// Replace explicitly changes an existing active deployment while retaining
// the previous definition in the historical catalog. It returns
// ErrDeploymentNotFound when no active route exists for the candidate name.
func (e *Engine) Replace(ctx context.Context, agent *core.Agent) (*Deployment, error) {
	return e.deploy(normalizeContext(ctx), agent, true)
}

func (e *Engine) deploy(ctx context.Context, agent *core.Agent, replace bool) (*Deployment, error) {
	operation := "Deploy"
	if replace {
		operation = "Replace"
	}
	deployment, err := e.compileAgent(agent)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.%s: %w", operation, err)
	}

	active, changed, err := e.catalog.activate(deployment, replace)
	if err != nil {
		return nil, fmt.Errorf("runtime.Engine.%s: %w", operation, err)
	}
	if changed {
		e.publishContext(ctx, event.AgentDeployed{
			Header:     event.NewHeader(""),
			Deployment: active.Ref(),
		})
	}
	return active, nil
}

// validateForDeploy validates the exact frozen definition that execution and
// durable identity will use, then runs every host extension validator against
// that same snapshot.
func (e *Engine) validateForDeploy(agent *core.Agent) error {
	if agent == nil {
		return errors.New("runtime.Engine.validateForDeploy: deploy agent: agent is nil")
	}

	var problems []error
	if err := validateAgentDefinition(agent); err != nil {
		problems = append(problems, err)
	}
	problems = append(problems, e.agentValidationErrors(agent)...)

	if joined := errors.Join(problems...); joined != nil {
		return fmt.Errorf("runtime.Engine.validateForDeploy: deploy agent %q: %w", agent.Name(), joined)
	}
	return nil
}

// Undeploy removes an agent. Returns an error when the name is
// unknown so callers don't silently miss typos.
func (e *Engine) Undeploy(ctx context.Context, name string) error {
	deployment, err := e.catalog.unregister(name)
	if err != nil {
		return fmt.Errorf("runtime.Engine.Undeploy: undeploy agent %q: %w", name, err)
	}
	e.publishContext(normalizeContext(ctx), event.AgentUndeployed{
		Header:     event.NewHeader(""),
		Deployment: deployment.Ref(),
	})
	return nil
}

// ActiveDeployment resolves the definition currently routed for name.
func (e *Engine) ActiveDeployment(name string) (*Deployment, bool) {
	return e.catalog.activeDeployment(name)
}

// Deployment resolves an exact active or historical definition.
func (e *Engine) Deployment(ref core.DeploymentRef) (*Deployment, bool) {
	return e.catalog.lookup(ref)
}

// ownedDeployment validates that deployment is one of this Engine's exact
// active or historical catalog entries. Advanced execution helpers use it to
// reject handles from another Engine instead of silently running a
// definition that cannot later be restored from this catalog.
func (e *Engine) ownedDeployment(operation string, deployment *Deployment) (*Deployment, error) {
	if e == nil {
		return nil, fmt.Errorf("runtime.%s: engine is nil", operation)
	}
	if deployment == nil || deployment.agent == nil {
		return nil, fmt.Errorf("runtime.%s: deployment is nil", operation)
	}
	owned, ok := e.catalog.lookup(deployment.Ref())
	if !ok || owned != deployment {
		return nil, fmt.Errorf("runtime.%s: %w: %s is not owned by this engine", operation, ErrDeploymentNotFound, deployment.Ref())
	}
	return owned, nil
}

// Deployments returns the complete active and historical catalog in stable
// Name, Version, Digest order. Deployment values are immutable.
func (e *Engine) Deployments() []*Deployment { return e.catalog.listAll() }

// ActiveDeployments returns current routes in stable agent-name order.
func (e *Engine) ActiveDeployments() []*Deployment { return e.catalog.listActive() }

// idGenerator returns the most-recently-registered IDGenerator
// extension, falling back to a UUID-v4 generator when none is
// registered.
func (e *Engine) idGenerator() extensionCapability[core.IDGenerator] {
	if generator, ok := lastExtension[core.IDGenerator](e.extensions.list); ok {
		return generator
	}
	return extensionCapability[core.IDGenerator]{name: core.UUIDGeneratorName, value: defaultIDGenerator}
}

// blackboardPrototype returns the most-recently-registered
// [core.Blackboard] extension. The runtime treats it as a
// prototype: every new process gets its own instance via
// [core.Blackboard.Clone] so per-process state stays isolated. Callers
// fall back to the in-memory implementation when no prototype is registered.
func (e *Engine) blackboardPrototype() (extensionCapability[core.Blackboard], bool) {
	return lastExtension[core.Blackboard](e.extensions.list)
}

// Built-in fallback for the IDGenerator singleton. Planner resolution
// uses [Engine.resolvePlanner] (name-based dispatch over registered
// extensions, with goap / reactive built-in defaults); Blackboard
// resolution uses [Engine.blackboardPrototype] + Clone().
var defaultIDGenerator = core.NewUUIDGenerator("")
