// Package sessionsearch is the session_search tool: full-text recall over the
// agent's past conversation transcripts. It is the "did we discuss X before"
// layer — a different corpus from memory_search (curated project memory) and
// codebase_search (source code): raw prior-session conversation, keyword-ranked.
package sessionsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/tools"
)

const (
	defaultLimit = 8
	maxLimit     = 20
)

type request struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"Keywords to recall from past conversations — a topic, decision, error, or approach (e.g. \"deploy the widget service\", \"rate limit bug\"). All terms must appear; exact phrasing is not required."`
	Limit int    `json:"limit,omitempty" jsonschema_description:"Maximum number of past-conversation excerpts to return (default 8)."`
}

func (r request) normalize() (request, error) {
	r.Query = strings.TrimSpace(r.Query)
	if r.Query == "" {
		return request{}, errors.New("query is required")
	}
	if r.Limit <= 0 {
		r.Limit = defaultLimit
	}
	if r.Limit > maxLimit {
		r.Limit = maxLimit
	}
	return r, nil
}

// Search is the transcript full-text search capability this tool consumes.
type Search interface {
	SearchTranscript(ctx context.Context, query string, limit int) ([]transcript.SearchHit, error)
}

type tool struct {
	search Search
}

// New builds the session_search tool over the given searcher. A nil searcher
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
		Name: "session_search",
		Description: "Search the full text of your PAST conversations — the transcripts of earlier sessions, what " +
			"you and the user actually said, across every prior session. Use it to recall whether a topic, decision, " +
			"error, or approach came up before (\"did we discuss X\", \"have I hit this bug\") instead of asking the " +
			"user to repeat themselves. Keyword search ranked by relevance; returns matching excerpts with who said it " +
			"and when. This is conversation history — not curated memory (memory_search) or source code (codebase_search / grep).",
	}
}

func (t *tool) run(ctx context.Context, req request) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", fmt.Errorf("session_search: %w", err)
	}
	hits, err := t.search.SearchTranscript(ctx, req.Query, req.Limit)
	if err != nil {
		return "", err
	}
	return results(hits).String(), nil
}

type results []transcript.SearchHit

func (r results) String() string {
	if len(r) == 0 {
		return "No earlier conversation matched. This topic may not have come up before."
	}
	var b strings.Builder
	for i, hit := range r {
		fmt.Fprintf(&b, "%d. [%s · %s] %s\n", i+1, speaker(hit.Kind), hit.CreatedAt.Format("2006-01-02"), strings.TrimSpace(hit.Snippet))
	}
	return strings.TrimRight(b.String(), "\n")
}

func speaker(kind transcript.ItemKind) string {
	if kind == transcript.UserMessage {
		return "user"
	}
	return "agent"
}
