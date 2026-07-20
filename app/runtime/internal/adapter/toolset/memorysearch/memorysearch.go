// Package memorysearch provides the memory_search tool — keyword + semantic
// search over the agent's own curated memory for the current project. One tool,
// one package. It is working-directory scoped (it searches the turn's project,
// read from the blackboard) but cwd-independent to build, so a single instance
// serves every session. It is offered whenever agent memory is enabled; keyword
// search works even when no embedding model is configured.
package memorysearch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

const defaultLimit = 8

type request struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"What you're trying to recall about this project — a topic, decision, convention, or preference (e.g. \"how do we run tests\", \"the user's naming preference\"). Natural language works; exact wording is not required."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum number of memories to return (default 8)."`
}

func (r request) normalize() (request, error) {
	r.Query = strings.TrimSpace(r.Query)
	if r.Query == "" {
		return request{}, errors.New("query is required")
	}
	if r.Limit <= 0 {
		r.Limit = defaultLimit
	}
	return r, nil
}

// Search is the agent-memory search capability this tool consumes.
type Search interface {
	Search(ctx context.Context, scope agentmemory.Scope, project, query string, topK int) ([]agentmemory.Item, error)
}

type tool struct {
	search Search
}

// New builds the memory_search tool over the given searcher. A nil searcher
// yields a nil tool (the feature is simply omitted), mirroring the other
// optional tools.
func New(search Search) (tools.Tool, error) {
	if search == nil {
		return nil, nil
	}
	return tools.New[request, string](definition(), (&tool{search: search}).run)
}

func definition() tools.Config {
	return tools.Config{
		Name: "memory_search",
		Description: "Search your own long-term memory of THIS project — the durable facts, conventions, " +
			"decisions, and user preferences you have accumulated across past sessions. Use it to recall context " +
			"that isn't already in the prompt before asking the user or re-deriving it. Ranks by relevance " +
			"(keyword + meaning). Returns the most relevant remembered notes.",
	}
}

func (t *tool) run(ctx context.Context, req request) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", fmt.Errorf("memory_search: %w", err)
	}
	cwd := strings.TrimSpace(turnctx.TurnCwd(ctx, ""))
	if cwd == "" {
		return "No project is associated with this session, so there is no project memory to search.", nil
	}
	items, err := t.search.Search(ctx, agentmemory.ScopeProject, filepath.Clean(cwd), req.Query, req.Limit)
	if err != nil {
		return "", err
	}
	return results(items).String(), nil
}

type results []agentmemory.Item

func (r results) String() string {
	if len(r) == 0 {
		return "No relevant memories found for this project. It may not have been recorded yet."
	}
	var b strings.Builder
	for i, item := range r {
		content := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(item.Content), "- "))
		fmt.Fprintf(&b, "%d. %s\n", i+1, content)
	}
	return strings.TrimRight(b.String(), "\n")
}
