package autonomy

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// Candidate is one (agent, goal) pair the autonomy router considers.
// The platform produces these by walking every deployed agent and
// pairing each with each of its goals.
type Candidate struct {
	Agent *core.Agent
	Goal  *core.Goal
}

// String renders "<agent>:<goal>" — used by the LLM prompt and by
// human-readable logging.
func (c Candidate) String() string {
	if c.Agent == nil || c.Goal == nil {
		return "<invalid candidate>"
	}
	return c.Agent.Name + ":" + c.Goal.Name
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
	Rank(ctx context.Context, userInput string, candidates []Candidate) ([]Choice, error)
}

// ErrNoConfidentChoice is returned by [Router.Choose] / [Router.Run]
// when the highest-scored candidate falls below
// [Config.GoalConfidenceCutOff]. Callers typically translate
// this into a "I don't know how to help with that" response or fall
// back to a default agent.
var ErrNoConfidentChoice = errors.New("autonomy: no candidate cleared the confidence cutoff")

// Config knobs the orchestrator. Zero value is usable: cutoff
// 0 (always pick the top score regardless of confidence), no agent
// filter, no extra approvers.
type Config struct {
	// GoalConfidenceCutOff is the minimum Confidence the top choice
	// must clear; otherwise [Router.Choose] returns
	// [ErrNoConfidentChoice]. 0 disables the gate.
	GoalConfidenceCutOff float64

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
	platform *runtime.Platform
	ranker   Ranker
	cfg      Config
}

// New returns an orchestrator backed by ranker. Both platform and
// ranker are required; nil returns an error — caller decides whether
// to surface or panic.
func New(p *runtime.Platform, ranker Ranker, cfg Config) (*Router, error) {
	if p == nil {
		return nil, errors.New("autonomy.New: platform must not be nil")
	}
	if ranker == nil {
		return nil, errors.New("autonomy.New: ranker must not be nil")
	}
	return &Router{platform: p, ranker: ranker, cfg: cfg}, nil
}

// Candidates enumerates the (agent, goal) pool currently visible to
// the orchestrator after AgentFilter / GoalFilter have run. Exposed
// so callers can inspect what the Ranker will see, e.g. for
// debugging or UI.
func (r *Router) Candidates() []Candidate {
	var out []Candidate
	for _, agent := range r.platform.Agents() {
		if agent == nil {
			continue
		}
		if r.cfg.AgentFilter != nil && !r.cfg.AgentFilter(agent) {
			continue
		}
		for _, goal := range agent.Goals {
			if goal == nil {
				continue
			}
			if r.cfg.GoalFilter != nil && !r.cfg.GoalFilter(agent, goal) {
				continue
			}
			out = append(out, Candidate{Agent: agent, Goal: goal})
		}
	}
	return out
}

// Choose ranks candidates against userInput and returns the top
// match, or [ErrNoConfidentChoice] when the top score is below the
// configured cutoff. Ties (equal Confidence) are broken by the
// Ranker's input order.
func (r *Router) Choose(ctx context.Context, userInput string) (Choice, error) {
	candidates := r.Candidates()
	if len(candidates) == 0 {
		return Choice{}, errors.New("autonomy.Router.Choose: no candidates available — deploy at least one agent first")
	}

	choices, err := r.ranker.Rank(ctx, userInput, candidates)
	if err != nil {
		return Choice{}, fmt.Errorf("autonomy.Router.Choose: %w", err)
	}
	if len(choices) == 0 {
		return Choice{}, errors.New("autonomy.Router.Choose: ranker returned no choices")
	}

	best := choices[0]
	for _, c := range choices[1:] {
		if c.Confidence > best.Confidence {
			best = c
		}
	}
	if best.Confidence < r.cfg.GoalConfidenceCutOff {
		return best, ErrNoConfidentChoice
	}
	return best, nil
}

// Run picks the best candidate for userInput and runs the chosen
// agent with bindings, locking the planner onto the chosen goal via
// a per-process [core.GoalApprover]. Returns the resulting process
// (terminal or waiting) and any error from Choose / RunAgent.
//
// On [ErrNoConfidentChoice] the returned process is nil; the caller
// can decide to fall back to a default agent or surface the failure
// to the user.
func (r *Router) Run(
	ctx context.Context,
	userInput string,
	bindings map[string]any,
	options core.ProcessOptions,
) (Choice, *runtime.AgentProcess, error) {
	choice, err := r.Choose(ctx, userInput)
	if err != nil {
		return choice, nil, err
	}

	options.Extensions = append(options.Extensions, &targetGoalApprover{
		name:       fmt.Sprintf("autonomy-target:%s", choice.Goal.Name),
		targetGoal: choice.Goal.Name,
	})

	proc, err := r.platform.RunAgent(ctx, choice.Agent, bindings, options)
	return choice, proc, err
}

// targetGoalApprover is the per-process [core.GoalApprover] Router
// installs to lock the planner onto the chosen goal. The runtime
// runs every approver on every goal-selection call; only the
// target name is approved and everything else is rejected.
type targetGoalApprover struct {
	name       string
	targetGoal string
}

func (a *targetGoalApprover) Name() string { return a.name }

func (a *targetGoalApprover) ApproveGoal(_ core.Process, goal *core.Goal) bool {
	return goal != nil && goal.Name == a.targetGoal
}
