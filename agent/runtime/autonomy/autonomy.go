// Package autonomy translates a natural-language user prompt into a
// concrete (agent, goal) decision and runs it.
//
// Two collaborating types:
//
//   - [Ranker] is the SPI: "given this user input, score each
//     candidate goal in [0, 1]". Plug an [LLMRanker] for chat-driven
//     ranking, a regex/keyword ranker for cheap routing, or a hybrid.
//   - [Autonomy] is the orchestrator: it enumerates the platform's
//     deployed agents × their goals, asks the Ranker, applies a
//     confidence cutoff, and (via [Autonomy.Run]) launches the
//     winning agent with a per-process [core.GoalApprover] that
//     locks the planner onto just the chosen goal.
//
// Mirrors embabel's `Autonomy` + `Ranker` SPI without the Spring DI
// scaffolding. lynx ships [LLMRanker] as the canonical LLM-backed
// ranker; users with simpler routing rules can implement [Ranker]
// directly.
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
// callers may rely on len(out) == len(candidates)). The Autonomy
// layer sorts and filters; Rankers don't need to.
type Ranker interface {
	Rank(ctx context.Context, userInput string, candidates []Candidate) ([]Choice, error)
}

// ErrNoConfidentChoice is returned by [Autonomy.Choose] / [Autonomy.Run]
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
	// must clear; otherwise [Autonomy.Choose] returns
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

// Autonomy is the orchestrator. Construct with [New].
type Autonomy struct {
	platform *runtime.Platform
	ranker   Ranker
	cfg      Config
}

// New returns an orchestrator backed by ranker. Both platform and
// ranker are required; nil returns an error — caller decides whether
// to surface or panic.
func New(platform *runtime.Platform, ranker Ranker, cfg Config) (*Autonomy, error) {
	if platform == nil {
		return nil, fmt.Errorf("autonomy.New: platform must not be nil")
	}
	if ranker == nil {
		return nil, fmt.Errorf("autonomy.New: ranker must not be nil")
	}
	return &Autonomy{platform: platform, ranker: ranker, cfg: cfg}, nil
}

// Candidates enumerates the (agent, goal) pool currently visible to
// the orchestrator after AgentFilter / GoalFilter have run. Exposed
// so callers can inspect what the Ranker will see, e.g. for
// debugging or UI.
func (a *Autonomy) Candidates() []Candidate {
	out := make([]Candidate, 0)
	for _, agent := range a.platform.Agents() {
		if agent == nil {
			continue
		}
		if a.cfg.AgentFilter != nil && !a.cfg.AgentFilter(agent) {
			continue
		}
		for _, goal := range agent.Goals {
			if goal == nil {
				continue
			}
			if a.cfg.GoalFilter != nil && !a.cfg.GoalFilter(agent, goal) {
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
func (a *Autonomy) Choose(ctx context.Context, userInput string) (Choice, error) {
	candidates := a.Candidates()
	if len(candidates) == 0 {
		return Choice{}, errors.New("autonomy.Autonomy.Choose: no candidates available — deploy at least one agent first")
	}

	choices, err := a.ranker.Rank(ctx, userInput, candidates)
	if err != nil {
		return Choice{}, fmt.Errorf("autonomy.Autonomy.Choose: %w", err)
	}
	if len(choices) == 0 {
		return Choice{}, errors.New("autonomy.Autonomy.Choose: ranker returned no choices")
	}

	best := choices[0]
	for _, c := range choices[1:] {
		if c.Confidence > best.Confidence {
			best = c
		}
	}
	if best.Confidence < a.cfg.GoalConfidenceCutOff {
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
func (a *Autonomy) Run(
	ctx context.Context,
	userInput string,
	bindings map[string]any,
	options core.ProcessOptions,
) (Choice, *runtime.AgentProcess, error) {
	choice, err := a.Choose(ctx, userInput)
	if err != nil {
		return choice, nil, err
	}

	options.Extensions = append(options.Extensions, &targetGoalApprover{
		name:       fmt.Sprintf("autonomy-target:%s", choice.Goal.Name),
		targetGoal: choice.Goal.Name,
	})

	proc, err := a.platform.RunAgent(ctx, choice.Agent, bindings, options)
	return choice, proc, err
}

// targetGoalApprover is the per-process [core.GoalApprover] Autonomy
// installs to lock the planner onto the chosen goal. The runtime
// runs every approver on every goal-selection call; we approve only
// the target name and reject everything else.
type targetGoalApprover struct {
	name       string
	targetGoal string
}

func (a *targetGoalApprover) Name() string { return a.name }

func (a *targetGoalApprover) ApproveGoal(_ core.Process, goal *core.Goal) bool {
	return goal != nil && goal.Name == a.targetGoal
}
