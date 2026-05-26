package autonomy

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/core/model/chat"
)

// PlanRanker reorders a slice of [*planning.Plan] by an arbitrary
// criterion. The default planner ranks by [plan.SortByNetValueDesc]
// (cost-vs-value math); a PlanRanker lets callers plug in
// LLM-driven, domain-aware, or hybrid ranking instead.
type PlanRanker interface {
	Rank(ctx context.Context, plans []*planning.Plan, ws core.WorldState) ([]*planning.Plan, error)
}

// LLMPlanRanker is a [PlanRanker] backed by [chat.Client]. The model
// sees a one-line summary of each plan (goal name, action sequence,
// cost, value) and returns a confidence-ordered list; the highest
// score wins.
type LLMPlanRanker struct {
	client *chat.Client
	cfg    LLMPlanRankerConfig
}

// LLMPlanRankerConfig knobs the prompt + parsing.
type LLMPlanRankerConfig struct {
	// SystemPrompt overrides the default classifier preamble.
	SystemPrompt string

	// PromptHeader is prefixed to the candidate listing in the user
	// message. Use to inject domain context the plan summaries don't
	// carry (recent user message, current world facts, …).
	PromptHeader string
}

// NewLLMPlanRanker constructs a ranker backed by client. Returns an
// error on a nil client — caller decides whether to surface or panic.
func NewLLMPlanRanker(client *chat.Client, cfg LLMPlanRankerConfig) (*LLMPlanRanker, error) {
	if client == nil {
		return nil, fmt.Errorf("autonomy.NewLLMPlanRanker: chat.Client must not be nil")
	}
	return &LLMPlanRanker{client: client, cfg: cfg}, nil
}

// Rank implements [PlanRanker]. Plans the LLM didn't score keep
// their original relative position (stable below the scored ones).
// Out-of-range / missing scores fall back to 0 so a botched reply
// fails closed.
func (r *LLMPlanRanker) Rank(ctx context.Context, plans []*planning.Plan, ws core.WorldState) ([]*planning.Plan, error) {
	if len(plans) < 2 {
		return plans, nil
	}

	systemPrompt := r.cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultPlanRankerSystemPrompt
	}

	userPrompt := r.buildUserPrompt(plans, ws)

	text, _, err := r.client.Chat().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Call().
		Text(ctx)
	if err != nil {
		return nil, fmt.Errorf("autonomy.LLMPlanRanker.Rank: %w", err)
	}

	scored, err := parseRankerReply(text)
	if err != nil {
		return nil, fmt.Errorf("autonomy.LLMPlanRanker.Rank: parse reply: %w (raw=%q)", err, text)
	}

	type ranked struct {
		plan  *planning.Plan
		score float64
	}
	scoredPlans := make([]ranked, len(plans))
	for i, p := range plans {
		s := 0.0
		if entry, ok := scored[planID(i, p)]; ok {
			s = clamp01(entry.Confidence)
		}
		scoredPlans[i] = ranked{plan: p, score: s}
	}

	slices.SortStableFunc(scoredPlans, func(a, b ranked) int {
		return cmp.Compare(b.score, a.score) // desc
	})

	out := make([]*planning.Plan, len(scoredPlans))
	for i, sp := range scoredPlans {
		out[i] = sp.plan
	}
	return out, nil
}

// buildUserPrompt renders the per-plan listing the LLM sees. The
// "id" we ask the model to echo back is "plan_<index>" so a duplicate
// goal name across plans doesn't collide.
func (r *LLMPlanRanker) buildUserPrompt(plans []*planning.Plan, ws core.WorldState) string {
	var b strings.Builder
	if header := r.cfg.PromptHeader; header != "" {
		b.WriteString(header)
		b.WriteString("\n\n")
	}
	b.WriteString("Candidate plans:\n")
	for i, p := range plans {
		fmt.Fprintf(&b, "- id=%s goal=%q value=%.2f cost=%.2f net=%.2f actions=[%s]\n",
			planID(i, p),
			planGoalName(p),
			p.Value(ws),
			p.Cost(ws),
			p.NetValue(ws),
			planActionList(p),
		)
	}
	b.WriteString(`
Score each plan's likelihood of producing a good outcome, on
[0.0, 1.0]. Reply with ONLY this JSON shape, no surrounding prose:

{"choices":[
  {"id":"plan_0","confidence":0.0,"rationale":"..."},
  ...
]}

Include every plan exactly once.
`)
	return b.String()
}

func planID(i int, _ *planning.Plan) string { return fmt.Sprintf("plan_%d", i) }

func planGoalName(p *planning.Plan) string {
	if p == nil || p.Goal == nil {
		return ""
	}
	return p.Goal.Name
}

func planActionList(p *planning.Plan) string {
	if p == nil || len(p.Actions) == 0 {
		return ""
	}
	names := make([]string, 0, len(p.Actions))
	for _, a := range p.Actions {
		if a == nil {
			continue
		}
		names = append(names, a.Metadata().Name)
	}
	return strings.Join(names, " → ")
}

const defaultPlanRankerSystemPrompt = `You are a planning evaluator. Given several candidate plans (each
a sequence of named actions toward a named goal, with cost / value
estimates), score how likely each plan is to deliver a useful
outcome.

Consider whether the goal looks aligned with prior context, whether
the actions cover the work needed, and whether the value/cost ratio
is favourable. Be strict: only mark 0.8+ when you would confidently
pick this plan as the best path forward.

Always reply with ONLY the JSON shape requested by the user message,
no markdown fences, no surrounding prose.`
