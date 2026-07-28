package routing

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

const (
	minimumConfidence = 0.0
	maximumConfidence = 1.0
	invalidCandidate  = "<invalid candidate>"
)

// Candidate is one (agent, goal) pair a Router considers.
// The engine produces these by walking every deployed agent and
// pairing each with each of its goals.
type Candidate struct {
	deployment core.DeploymentRef
	agent      core.AgentDescriptor
	goal       core.GoalDescriptor
}

func newCandidate(deployment core.DeploymentRef, agent core.AgentDescriptor, goal core.GoalDescriptor) Candidate {
	return Candidate{deployment: deployment, agent: agent, goal: goal}
}

// Deployment returns the exact immutable definition identity being ranked.
func (c Candidate) Deployment() core.DeploymentRef { return c.deployment }

// Agent returns the non-executable agent description being ranked.
func (c Candidate) Agent() core.AgentDescriptor { return c.agent }

// Goal returns the non-executable target being ranked.
func (c Candidate) Goal() core.GoalDescriptor { return c.goal }

// String renders "<agent>:<goal>" — used by the LLM prompt and by
// human-readable logging.
func (c Candidate) String() string {
	if c.goal.Name() == "" {
		return invalidCandidate
	}
	name := c.deployment.Name
	if name == "" {
		name = c.agent.Name()
	}
	if name == "" {
		return invalidCandidate
	}
	return name + ":" + c.goal.Name()
}

// Choice is a Candidate plus the Ranker's verdict on it. Confidence is finite
// and lives in [0, 1]; 0 = irrelevant, 1 = perfect match. Rationale is
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

// Runtime is the deployment surface Router consumes: it enumerates the routes
// currently active. Selection needs nothing else — a Choice names an exact
// immutable identity, and running it is the caller's step, with
// [runtime.Engine.RunDeployment] or however else it drives the engine.
type Runtime interface {
	ActiveDeployments() []*runtime.Deployment
}

// ErrNoMatch is returned by [Router.Choose] when the highest-scored candidate
// falls below
// [Config.MinConfidence]. Callers typically translate
// this into a "I don't know how to help with that" response or fall
// back to a default agent.
var ErrNoMatch = errors.New("routing: no candidate cleared the confidence threshold")

// Config knobs selection. Zero value is usable: cutoff 0 (always pick the top
// score regardless of confidence) and no filtering.
type Config struct {
	// MinConfidence is the minimum confidence the top choice
	// must clear; otherwise [Router.Choose] returns
	// [ErrNoMatch]. 0 disables the gate.
	MinConfidence float64

	// AgentFilter, when non-nil, restricts the candidate pool to agents the
	// predicate returns true for. Why a caller narrows the pool is its own
	// concern; the router only applies the predicate.
	AgentFilter func(core.AgentDescriptor) bool

	// GoalFilter, when non-nil, restricts which goals on each
	// surviving agent become candidates.
	GoalFilter func(core.AgentDescriptor, core.GoalDescriptor) bool
}

// Router is the orchestrator. Construct with [New].
type Router struct {
	runtime Runtime
	ranker  Ranker
	config  Config
}

// New returns an orchestrator backed by ranker. Both runtime and
// ranker are required; nil returns an error — caller decides whether
// to surface or panic.
func New(agentRuntime Runtime, ranker Ranker, config Config) (*Router, error) {
	if agentRuntime == nil {
		return nil, errors.New("routing: runtime is nil")
	}
	if ranker == nil {
		return nil, errors.New("routing: ranker is nil")
	}
	if math.IsNaN(config.MinConfidence) || config.MinConfidence < minimumConfidence || config.MinConfidence > maximumConfidence {
		return nil, errors.New("routing: minimum confidence must be between 0 and 1")
	}
	return &Router{runtime: agentRuntime, ranker: ranker, config: config}, nil
}

// Candidates enumerates the (agent, goal) pool currently visible to
// the orchestrator after AgentFilter / GoalFilter have run. Exposed
// so callers can inspect what the Ranker will see, e.g. for
// debugging or UI.
func (r *Router) Candidates() []Candidate {
	var candidates []Candidate
	for _, deployment := range r.runtime.ActiveDeployments() {
		// Runtime is a consumer-defined port, so the deployment list is only as
		// well-formed as its implementation. A deployment that reached the engine
		// carries a validated agent, which is why nothing here re-checks the
		// description it projects.
		if deployment == nil {
			continue
		}
		agent := deployment.Descriptor()
		if r.config.AgentFilter != nil && !r.config.AgentFilter(agent) {
			continue
		}
		for _, goal := range agent.Goals() {
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
		if choices[i].Deployment() != candidates[i].Deployment() ||
			choices[i].Goal().Name() != candidates[i].Goal().Name() {
			return Choice{}, fmt.Errorf("routing: ranker changed candidate at index %d", i)
		}
		if !validConfidence(choices[i].Confidence) {
			return Choice{}, fmt.Errorf("routing: ranker returned invalid confidence %v at index %d; confidence must be finite and between 0 and 1", choices[i].Confidence, i)
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

func validConfidence(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimumConfidence && value <= maximumConfidence
}
