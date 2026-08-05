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

// String renders "<agent>:<goal>". A Candidate a caller built by hand, rather
// than through [Router.Candidates], renders as a placeholder instead of a
// half-formed identity.
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

// Ranker scores how well each Candidate matches the input. It MUST return one
// [Choice] per candidate, positionally aligned — the Router verifies that and
// rejects a ranker that reorders or drops one. Sorting and filtering belong to
// the Router, not here.
type Ranker interface {
	Rank(ctx context.Context, input string, candidates []Candidate) ([]Choice, error)
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
	engine *runtime.Engine
	ranker Ranker
	config Config
}

// New returns a router over the engine's active deployment catalog. The engine
// only supplies immutable routing candidates; running the selected deployment
// remains the caller's explicit next step.
func New(engine *runtime.Engine, ranker Ranker, config Config) (*Router, error) {
	if engine == nil {
		return nil, errors.New("routing: engine is nil")
	}
	if ranker == nil {
		return nil, errors.New("routing: ranker is nil")
	}
	if math.IsNaN(config.MinConfidence) || config.MinConfidence < minimumConfidence || config.MinConfidence > maximumConfidence {
		return nil, errors.New("routing: minimum confidence must be between 0 and 1")
	}
	return &Router{engine: engine, ranker: ranker, config: config}, nil
}

// Candidates enumerates the (agent, goal) pool left after AgentFilter and
// GoalFilter have run — exactly what [Router.Choose] will hand the Ranker.
func (r *Router) Candidates() []Candidate {
	var candidates []Candidate
	for _, deployment := range r.engine.ActiveDeployments() {
		// Every deployment in the engine catalog already carries a validated,
		// immutable definition, so routing only projects its descriptors.
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

// Choose ranks the candidates against input and returns the top match, or
// [ErrNoMatch] when the top score is below the configured cutoff. Ties are
// broken by the order the Ranker received them, so an indecisive ranker still
// routes deterministically.
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
