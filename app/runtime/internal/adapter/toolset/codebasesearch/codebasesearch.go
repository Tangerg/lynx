// Package codebasesearch provides the codebase_search tool — semantic search
// over the project's code via the @codebase index. One tool, one package. It is
// working-directory scoped (it searches the turn's cwd, read from the
// blackboard) but cwd-independent to build, so a single instance serves every
// session; it's offered only when an embedding model is configured.
package codebasesearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turnctx"
)

// maxSnippetLines caps how much of each matched chunk the result shows — enough
// to judge relevance; the agent reads the file for full context.
const maxSnippetLines = 12

const inputSchema = `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "A natural-language description of the code you're looking for — a concept, behavior, or where something is implemented (e.g. \"where retries are configured\", \"the websocket reconnect logic\"). For an exact string or symbol, use grep instead."
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of results to return (default 8)."
    }
  },
  "required": ["query"]
}`

type request struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type tool struct {
	index codebaseindex.Index
}

// New builds the codebase_search tool over the given index.
func New(index codebaseindex.Index) chat.Tool {
	return &tool{index: index}
}

func (t *tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "codebase_search",
		Description: "Semantic search over THIS project's code: find the most relevant code by MEANING, not by literal text. " +
			"Use it to locate where a concept or behavior lives when you don't know the exact name; use grep for exact strings/symbols. " +
			"Returns ranked file:line snippets. The index builds on first use and refreshes as files change.",
		InputSchema: inputSchema,
	}
}

func (t *tool) Call(ctx context.Context, arguments string) (string, error) {
	var req request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("codebase_search: invalid arguments: %w", err)
	}
	if strings.TrimSpace(req.Query) == "" {
		return "", errors.New("codebase_search: query is required")
	}
	hits, err := t.index.Search(ctx, turnctx.TurnCwd(ctx, ""), req.Query, req.Limit)
	if err != nil {
		if errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
			return "", errors.New("codebase_search: no embedding model is configured — set one in Settings → Models (an embedding-capable provider like OpenAI, or a local Ollama)")
		}
		return "", err
	}
	return render(hits), nil
}

func render(hits []codebaseindex.Hit) string {
	if len(hits) == 0 {
		return "No semantically similar code found. Try rephrasing the query, or use grep for an exact string."
	}
	var b strings.Builder
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s:%d-%d  (score %.2f)\n", i+1, h.Path, h.StartLine, h.EndLine, h.Score)
		b.WriteString(snippet(h.Text))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// snippet trims a chunk to maxSnippetLines so the result stays scannable.
func snippet(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxSnippetLines {
		return text
	}
	return strings.Join(lines[:maxSnippetLines], "\n") + "\n…"
}
