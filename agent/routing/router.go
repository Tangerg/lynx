package routing

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Candidate is one (agent, goal) pair a Router considers.
// The engine produces these by walking every deployed agent and
// pairing each with each of its goals.
type Candidate struct {
	deployment core.DeploymentRef
	agent      *core.Agent
	goal       *core.Goal
}

func newCandidate(deployment core.DeploymentRef, agent *core.Agent, goal *core.Goal) Candidate {
	return Candidate{deployment: deployment, agent: agent, goal: goal}
}

// Deployment returns the exact immutable definition identity being ranked.
func (c Candidate) Deployment() core.DeploymentRef { return c.deployment }

// Agent returns the immutable agent definition being ranked.
func (c Candidate) Agent() *core.Agent { return c.agent }

// Goal returns the immutable target being ranked.
func (c Candidate) Goal() *core.Goal { return c.goal }

// String renders "<agent>:<goal>" — used by the LLM prompt and by
// human-readable logging.
func (c Candidate) String() string {
	if c.goal == nil {
		return "<invalid candidate>"
	}
	name := c.deployment.Name
	if name == "" && c.agent != nil {
		name = c.agent.Name()
	}
	if name == "" {
		return "<invalid candidate>"
	}
	return name + ":" + c.goal.Name()
}

// Choice is a Candidate plus the Ranker's verdict on it. Confidence
// lives in [0, 1]; 0 = irrelevant, 1 = perfect match. Rationale is
// optional human-readable text the Ranker may attach.
type Choice struct {
	Candidate
	Confidence float64
	Rationale  string
}

// Ranker scores how well each Candidate matches userInput. It MUST
// return one [Choice] per input candidate (positionally aligned;
// callers may rely on len(out) == len(candidates)). The Router
// layer sorts and filters; Rankers don't need to.
type Ranker interface {
	Rank(ctx context.Context, input string, candidates []Candidate) ([]Choice, error)
}

// ErrNoMatch is returned by [Router.Choose] / [Router.Run]
// when the highest-scored candidate falls below
// [Config.MinConfidence]. Callers typically translate
// this into a "I don't know how to help with that" response or fall
// back to a default agent.
var ErrNoMatch = errors.New("routing: no candidate cleared the confidence threshold")

// Config knobs the orchestrator. Zero value is usable: cutoff
// 0 (always pick the top score regardless of confidence), no agent
// filter, no extra approvers.
type Config struct {
	// MinConfidence is the minimum confidence the top choice
	// must clear; otherwise [Router.Choose] returns
	// [ErrNoMatch]. 0 disables the gate.
	MinConfidence float64

	// AgentFilter, when non-nil, restricts the candidate pool to
	// agents the predicate returns true for. Use for tenant
	// isolation, role-based access, or hiding internal agents.
	AgentFilter func(*core.Agent) bool

	// GoalFilter, when non-nil, restricts which goals on each
	// surviving agent become candidates.
	GoalFilter func(*core.Agent, *core.Goal) bool
}

// Router is the orchestrator. Construct with [New].
type Router struct {
	engine *runtime.Engine
	ranker Ranker
	config Config
}

// New returns an orchestrator backed by ranker. Both engine and
// ranker are required; nil returns an error — caller decides whether
// to surface or panic.
func New(engine *runtime.Engine, ranker Ranker, config Config) (*Router, error) {
	if engine == nil {
		return nil, errors.New("routing: engine is nil")
	}
	if ranker == nil {
		return nil, errors.New("routing: ranker is nil")
	}
	if math.IsNaN(config.MinConfidence) || config.MinConfidence < 0 || config.MinConfidence > 1 {
		return nil, errors.New("routing: minimum confidence must be between 0 and 1")
	}
	return &Router{engine: engine, ranker: ranker, config: config}, nil
}

// Candidates enumerates the (agent, goal) pool currently visible to
// the orchestrator after AgentFilter / GoalFilter have run. Exposed
// so callers can inspect what the Ranker will see, e.g. for
// debugging or UI.
func (r *Router) Candidates() []Candidate {
	var candidates []Candidate
	for _, deployment := range r.engine.ActiveDeployments() {
		agent := deployment.Agent()
		if agent == nil {
			continue
		}
		if r.config.AgentFilter != nil && !r.config.AgentFilter(agent) {
			continue
		}
		for _, goal := range agent.Goals() {
			if goal == nil {
				continue
			}
			if r.config.GoalFilter != nil && !r.config.GoalFilter(agent, goal) {
				continue
			}
			candidates = append(candidates, newCandidate(deployment.Ref(), agent, goal))
		}
	}
	return candidates
}

// Choose ranks candidates against userInput and returns the top
// match, or [ErrNoMatch] when the top score is below the
// configured cutoff. Ties (equal Confidence) are broken by the
// Ranker's input order.
func (r *Router) Choose(ctx context.Context, input string) (Choice, error) {
	candidates := r.Candidates()
	if len(candidates) == 0 {
		return Choice{}, ErrNoMatch
	}

	choices, err := r.ranker.Rank(ctx, input, candidates)
	if err != nil {
		return Choice{}, fmt.Errorf("routing: rank candidates: %w", err)
	}
	if len(choices) != len(candidates) {
		return Choice{}, fmt.Errorf("routing: ranker returned %d choices for %d candidates", len(choices), len(candidates))
	}
	for i := range candidates {
		if choices[i].Deployment() != candidates[i].Deployment() || choices[i].Goal() == nil ||
			choices[i].Goal().Name() != candidates[i].Goal().Name() {
			return Choice{}, fmt.Errorf("routing: ranker changed candidate at index %d", i)
		}
	}

	best := choices[0]
	for _, choice := range choices[1:] {
		if choice.Confidence > best.Confidence {
			best = choice
		}
	}
	if best.Confidence < r.config.MinConfidence {
		return best, ErrNoMatch
	}
	return best, nil
}

// Run picks the best candidate for userInput and runs the chosen
// agent with bindings, locking the planner onto the chosen goal via
// a per-process [core.GoalApprover]. Returns the resulting process
// (terminal or waiting) and any error from Choose / Run.
//
// On [ErrNoMatch] the returned process is nil; the caller
// can decide to fall back to a default agent or surface the failure
// to the user.
func (r *Router) Run(
	ctx context.Context,
	input string,
	bindings map[string]any,
	options core.ProcessOptions,
) (Choice, *runtime.Process, error) {
	choice, err := r.Choose(ctx, input)
	if err != nil {
		return choice, nil, err
	}

	goal := choice.Goal()
	options.Extensions = append(options.Extensions, &targetGoalApprover{
		name:     fmt.Sprintf("routing-target:%s", goal.Name()),
		goalName: goal.Name(),
	})

	deploymentRef := choice.Deployment()
	deployment, ok := r.engine.Deployment(deploymentRef)
	if !ok {
		return choice, nil, fmt.Errorf("routing: deployment %s is no longer available", deploymentRef)
	}
	process, err := r.engine.Run(ctx, deployment.Agent(), bindings, options)
	if process != nil && process.Deployment() != deploymentRef {
		return choice, process, fmt.Errorf("routing: process bound %s, want %s", process.Deployment(), deploymentRef)
	}
	return choice, process, err
}

// targetGoalApprover is the per-process [core.GoalApprover] Router
// installs to lock the planner onto the chosen goal. The runtime
// runs every approver on every goal-selection call; only the
// target name is approved and everything else is rejected.
type targetGoalApprover struct {
	name     string
	goalName string
}

func (a *targetGoalApprover) Name() string { return a.name }

func (a *targetGoalApprover) Approve(_ core.ProcessView, goal *core.Goal) bool {
	return goal != nil && goal.Name() == a.goalName
}
