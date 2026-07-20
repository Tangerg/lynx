package routing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

// ModelRanker asks a chat model to score each routing candidate.
type ModelRanker struct {
	model  chat.Model
	config ModelConfig
}

// ModelConfig configures a [ModelRanker]. Its zero value is usable.
type ModelConfig struct {
	// SystemPrompt overrides the default "you are a routing
	// classifier" preamble. Use for domain-specific guidance ("score
	// strictly; favor exact-match keyword overlap").
	SystemPrompt string

	// PromptHeader prefixes the per-candidate listing in the user
	// message. Use to surface domain context the LLM needs but the
	// candidate descriptions don't carry. Default: empty.
	PromptHeader string
}

// NewModelRanker returns a ranker backed by model.
func NewModelRanker(model chat.Model, config ModelConfig) (*ModelRanker, error) {
	if model == nil {
		return nil, errors.New("routing: model is nil")
	}
	return &ModelRanker{model: model, config: config}, nil
}

// Rank implements [Ranker]. Returns one [Choice] per input
// candidate, with Confidence in [0, 1] and an optional rationale.
// Candidates the LLM omitted from its reply default to confidence 0.
func (r *ModelRanker) Rank(ctx context.Context, userInput string, candidates []Candidate) ([]Choice, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	systemPrompt := r.config.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultRankerSystemPrompt
	}

	userPrompt := r.userPrompt(userInput, candidates)

	request, err := chat.NewRequest(
		chat.NewSystemMessage(systemPrompt),
		chat.NewUserMessage(chat.NewTextPart(userPrompt)),
	)
	if err != nil {
		return nil, fmt.Errorf("routing: build ranking request: %w", err)
	}
	response, err := r.model.Call(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("routing: rank candidates: %w", err)
	}
	if response == nil {
		return nil, errors.New("routing: model returned a nil response")
	}
	text := response.Text()

	scored, err := parseRankResponse(text)
	if err != nil {
		return nil, fmt.Errorf("routing: parse ranking response: %w (raw=%q)", err, text)
	}

	choices := make([]Choice, len(candidates))
	for index, candidate := range candidates {
		choices[index] = Choice{Candidate: candidate}
		key := candidate.String()
		if entry, ok := scored[key]; ok {
			choices[index].Confidence = max(minimumConfidence, min(maximumConfidence, entry.Confidence))
			choices[index].Rationale = entry.Rationale
		}
	}
	return choices, nil
}

// userPrompt composes the candidate listing the model sees. Each
// row is "<agent>:<goal> — <description>" plus optional Tags /
// Examples blocks pulled from [core.Goal]; the trailing instruction
// pins the JSON format the ranker parses.
func (r *ModelRanker) userPrompt(userInput string, candidates []Candidate) string {
	var builder strings.Builder
	if header := r.config.PromptHeader; header != "" {
		builder.WriteString(header)
		builder.WriteString("\n\n")
	}
	builder.WriteString("User said: ")
	builder.WriteString(strconv.Quote(userInput))
	builder.WriteString("\n\nCandidates:\n")
	for _, candidate := range candidates {
		fmt.Fprintf(&builder, "- %s — %s\n", candidate.String(), candidate.goalDescription())
		candidate.writeGoalHints(&builder)
	}
	builder.WriteString(`
Score each candidate's relevance to the user input on [0.0, 1.0]
(0.0 = irrelevant, 1.0 = perfect match). Reply with ONLY a JSON
object, no surrounding prose, in this exact shape:

{"choices":[
  {"id":"<agent>:<goal>","confidence":0.0,"rationale":"..."},
  ...
]}

Include every candidate exactly once. confidence must be a number.
`)
	return builder.String()
}

// writeGoalHints emits the optional Tags / Examples blocks on
// indented continuation lines so the LLM has a richer match signal
// than Name+Description alone. No-op when both are empty.
func (c Candidate) writeGoalHints(builder *strings.Builder) {
	goal := c.Goal()
	if goal == nil {
		return
	}
	if tags := goal.Tags(); len(tags) > 0 {
		fmt.Fprintf(builder, "    tags: %s\n", strings.Join(tags, ", "))
	}
	if examples := goal.Examples(); len(examples) > 0 {
		builder.WriteString("    examples:\n")
		for _, example := range examples {
			fmt.Fprintf(builder, "      - %s\n", strconv.Quote(example))
		}
	}
}

// goalDescription returns the goal's Description, falling back to
// the agent's when the goal didn't supply one. Empty as a last
// resort — the LLM still sees the name.
func (c Candidate) goalDescription() string {
	goal := c.Goal()
	if goal == nil {
		return ""
	}
	if description := goal.Description(); description != "" {
		return description
	}
	if agent := c.Agent(); agent != nil {
		return agent.Description()
	}
	return ""
}

type rankedCandidate struct {
	ID         string  `json:"id"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type rankResponse struct {
	Choices []rankedCandidate `json:"choices"`
}

func parseRankResponse(text string) (map[string]rankedCandidate, error) {
	object := jsonObject(text)
	if object == "" {
		return nil, errors.New("no JSON object in response")
	}
	var response rankResponse
	if err := json.Unmarshal([]byte(object), &response); err != nil {
		return nil, err
	}
	choices := make(map[string]rankedCandidate, len(response.Choices))
	for _, choice := range response.Choices {
		if choice.ID == "" {
			continue
		}
		choices[choice.ID] = choice
	}
	return choices, nil
}

func jsonObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

const defaultRankerSystemPrompt = `You are a routing classifier. Given a user request and a list of
named candidate goals (each "agent:goal — description"), score how
well each candidate matches the user's intent.

Be strict: irrelevant goals score 0.0; a tangentially-related goal
should score below 0.5; only mark 0.8+ when the goal directly
addresses the user's request.

Always reply with ONLY the JSON shape requested by the user message,
no markdown fences, no surrounding prose.`
