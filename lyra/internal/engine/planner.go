package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// planner is the LLM-produced-plan generator. Distinct from lynx's
// algorithmic planners (GOAP / HTN / utility / reactive): those
// search a known Action space, this one asks the language model to
// describe what it intends to do in natural language so the user
// can audit / approve before any tool runs.
//
// Direct chat.Client.Chat() call — bypasses the chat-memory
// middleware so plan generation doesn't pollute the conversation
// history (the plan itself goes into history only on Approve, when
// the regular execution path runs).
type planner struct {
	client *chat.Client
}

func newPlanner(client *chat.Client) *planner {
	if client == nil {
		return nil
	}
	return &planner{client: client}
}

// Plan asks the LLM for a step-by-step plan to handle userMessage,
// optionally seeded with the system prompt the regular agent would
// have used (so plan generation sees the same LYRA.md context).
// Returns the raw markdown — the runtime emits it verbatim as
// [PlanGenerated.Plan].
//
// Failure means the runtime falls back to direct execution rather
// than blocking on a missing plan — the caller (chat.Service)
// decides whether to surface the error or proceed.
func (p *planner) Plan(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if p == nil {
		return "", fmt.Errorf("planner: chat client missing")
	}

	const planInstructions = `Before doing anything, draft a brief plan
for handling the user's request. Format as a numbered markdown
list (5 steps max). Each step should be a single concrete action
— "read X", "edit Y to do Z", "run the tests" — not a generic
phrase.

If the request is trivial (e.g. just answering a question) reply
with exactly: NO_PLAN

Output ONLY the plan or NO_PLAN — no preamble, no closing
commentary, no JSON wrapper.`

	composedSystem := planInstructions
	if strings.TrimSpace(systemPrompt) != "" {
		composedSystem = systemPrompt + "\n\n---\n\n" + planInstructions
	}

	text, err := askDirect(ctx, p.client, composedSystem, userMessage)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || trimmed == "NO_PLAN" {
		return "", nil
	}
	return trimmed, nil
}
