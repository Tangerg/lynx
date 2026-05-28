package autonomy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// LLMRanker is a [Ranker] that asks an LLM to score each candidate
// against userInput. The model is prompted to return a strict JSON
// object; missing or out-of-range scores fall back to 0 so a
// misbehaving model fails closed (= "irrelevant") rather than
// hijacking selection.
type LLMRanker struct {
	client *chat.Client
	cfg    LLMRankerConfig
}

// LLMRankerConfig knobs prompt + parsing. Zero value is usable: a
// built-in prompt that asks for a confidence + rationale per
// candidate.
type LLMRankerConfig struct {
	// SystemPrompt overrides the default "you are a routing
	// classifier" preamble. Use for domain-specific guidance ("score
	// strictly; favour exact-match keyword overlap").
	SystemPrompt string

	// PromptHeader prefixes the per-candidate listing in the user
	// message. Use to surface domain context the LLM needs but the
	// candidate descriptions don't carry. Default: empty.
	PromptHeader string
}

// NewLLMRanker constructs a ranker backed by client. Returns an error
// on a nil client — caller decides whether to surface or panic.
func NewLLMRanker(client *chat.Client, cfg LLMRankerConfig) (*LLMRanker, error) {
	if client == nil {
		return nil, errors.New("autonomy.NewLLMRanker: chat.Client must not be nil")
	}
	return &LLMRanker{client: client, cfg: cfg}, nil
}

// Rank implements [Ranker]. Returns one [Choice] per input
// candidate, with Confidence in [0, 1] and an optional rationale.
// Candidates the LLM omitted from its reply default to confidence 0.
func (r *LLMRanker) Rank(ctx context.Context, userInput string, candidates []Candidate) ([]Choice, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	systemPrompt := r.cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultRankerSystemPrompt
	}

	userPrompt := r.buildUserPrompt(userInput, candidates)

	text, _, err := r.client.Chat().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Call().
		Text(ctx)
	if err != nil {
		return nil, fmt.Errorf("autonomy.LLMRanker.Rank: %w", err)
	}

	scored, err := parseRankerReply(text)
	if err != nil {
		return nil, fmt.Errorf("autonomy.LLMRanker.Rank: parse reply: %w (raw=%q)", err, text)
	}

	out := make([]Choice, len(candidates))
	for i, cand := range candidates {
		out[i] = Choice{Candidate: cand}
		key := cand.String()
		if entry, ok := scored[key]; ok {
			out[i].Confidence = clamp01(entry.Confidence)
			out[i].Rationale = entry.Rationale
		}
	}
	return out, nil
}

// buildUserPrompt composes the candidate listing the LLM sees. Each
// row is "<agent>:<goal> — <description>" plus optional Tags /
// Examples blocks pulled from [core.Goal]; the trailing instruction
// pins the JSON format we'll parse.
func (r *LLMRanker) buildUserPrompt(userInput string, candidates []Candidate) string {
	var b strings.Builder
	if header := r.cfg.PromptHeader; header != "" {
		b.WriteString(header)
		b.WriteString("\n\n")
	}
	b.WriteString("User said: ")
	b.WriteString(strconv.Quote(userInput))
	b.WriteString("\n\nCandidates:\n")
	for _, cand := range candidates {
		fmt.Fprintf(&b, "- %s — %s\n", cand.String(), goalDescription(cand))
		writeGoalHints(&b, cand)
	}
	b.WriteString(`
Score each candidate's relevance to the user input on [0.0, 1.0]
(0.0 = irrelevant, 1.0 = perfect match). Reply with ONLY a JSON
object, no surrounding prose, in this exact shape:

{"choices":[
  {"id":"<agent>:<goal>","confidence":0.0,"rationale":"..."},
  ...
]}

Include every candidate exactly once. confidence must be a number.
`)
	return b.String()
}

// writeGoalHints emits the optional Tags / Examples blocks on
// indented continuation lines so the LLM has a richer match signal
// than Name+Description alone. No-op when both are empty.
func writeGoalHints(b *strings.Builder, cand Candidate) {
	if cand.Goal == nil {
		return
	}
	if len(cand.Goal.Tags) > 0 {
		fmt.Fprintf(b, "    tags: %s\n", strings.Join(cand.Goal.Tags, ", "))
	}
	if len(cand.Goal.Examples) > 0 {
		b.WriteString("    examples:\n")
		for _, ex := range cand.Goal.Examples {
			fmt.Fprintf(b, "      - %s\n", strconv.Quote(ex))
		}
	}
}

// goalDescription returns the goal's Description, falling back to
// the agent's when the goal didn't supply one. Empty as a last
// resort — the LLM still sees the name.
func goalDescription(cand Candidate) string {
	if cand.Goal == nil {
		return ""
	}
	if cand.Goal.Description != "" {
		return cand.Goal.Description
	}
	if cand.Agent != nil {
		return cand.Agent.Description
	}
	return ""
}

// rankerEntry mirrors one element of the model's JSON reply.
type rankerEntry struct {
	ID         string  `json:"id"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type rankerReply struct {
	Choices []rankerEntry `json:"choices"`
}

// parseRankerReply extracts the JSON object from text (LLMs often
// wrap their JSON in prose) and returns id → entry. Missing /
// malformed entries surface as a parse error rather than silently
// scoring 0 — a botched reply should be observable.
func parseRankerReply(text string) (map[string]rankerEntry, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, errors.New("autonomy.parseRankerReply: no JSON object in reply")
	}
	var reply rankerReply
	if err := json.Unmarshal([]byte(jsonStr), &reply); err != nil {
		return nil, err
	}
	out := make(map[string]rankerEntry, len(reply.Choices))
	for _, e := range reply.Choices {
		if e.ID == "" {
			continue
		}
		out[e.ID] = e
	}
	return out, nil
}

// extractJSON pulls the first balanced top-level "{...}" object out
// of text — robustness for LLMs that prepend "Here is the JSON:" or
// trail with markdown fences.
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	}
	return v
}

const defaultRankerSystemPrompt = `You are a routing classifier. Given a user request and a list of
named candidate goals (each "agent:goal — description"), score how
well each candidate matches the user's intent.

Be strict: irrelevant goals score 0.0; a tangentially-related goal
should score below 0.5; only mark 0.8+ when the goal directly
addresses the user's request.

Always reply with ONLY the JSON shape requested by the user message,
no markdown fences, no surrounding prose.`
