// Package sessionsearch exposes search_conversations over prior transcripts.
package sessionsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

const defaultLimit = 8

type request struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Keywords that should appear in earlier conversation transcripts."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum matching excerpts to return. Defaults to 8."`
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

// Search is the transcript full-text search capability this tool consumes.
type Search interface {
	SearchTranscript(ctx context.Context, query string, limit int) ([]transcript.SearchHit, error)
}

type searcher struct {
	search Search
}

// New builds search_conversations over the given searcher. A nil searcher
// yields a nil tool (the feature is simply omitted), mirroring the other
// optional tools.
func New(search Search) (toolcontract.Tool, error) {
	if search == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[request, string](definition(), (&searcher{search: search}).run)
}

func definition() toolcontract.FuncConfig {
	return toolcontract.FuncConfig{
		Name: "search_conversations",
		Description: "Search raw transcripts from earlier conversations by keyword and return matching excerpts " +
			"with speaker and date. Use it to determine whether a topic, decision, error, or approach was discussed " +
			"before. Use search_memory for curated durable facts and grep for source code.",
	}
}

func (t *searcher) run(ctx context.Context, req request) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", fmt.Errorf("search_conversations: %w", err)
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
