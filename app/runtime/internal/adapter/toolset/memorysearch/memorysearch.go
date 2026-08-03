// Package memorysearch provides the search_memory tool — keyword + semantic
// search over the agent's own curated memory for the current project. One tool,
// one package. It is working-directory scoped (it searches the turn's project,
// read from application context) but cwd-independent to build, so a single instance
// serves every session. It is offered whenever agent memory is enabled; keyword
// search works even when no embedding model is configured.
package memorysearch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

const defaultLimit = 8

type request struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Natural-language topic, decision, convention, or user preference to recall from curated project memory."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum memories to return. Defaults to 8."`
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

// New builds search_memory over the given searcher. A nil searcher
// yields a nil tool (the feature is simply omitted), mirroring the other
// optional tools.
func New(search Search) (toolcontract.Tool, error) {
	if search == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[request, string](definition(), (&tool{search: search}).run)
}

func definition() toolcontract.FuncConfig {
	return toolcontract.FuncConfig{
		Name: "search_memory",
		Description: "Search curated long-term memory for the current project, including durable decisions, " +
			"conventions, and user preferences from earlier work. Use it when needed context is not already in " +
			"the prompt. This is distilled memory, not raw conversation history; use search_conversations to " +
			"recall what was said.",
	}
}

func (t *tool) run(ctx context.Context, req request) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", fmt.Errorf("search_memory: %w", err)
	}
	cwd := strings.TrimSpace(executionctx.CWD(ctx, ""))
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
