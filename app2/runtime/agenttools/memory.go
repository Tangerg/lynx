package agenttools

import (
	"context"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/agentexec"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	defaultMemorySearchLimit = 8
	maxMemorySearchLimit     = 10
	maxMemorySearchOutput    = 32 << 10
)

type MemoryHit struct {
	Scope   string
	Content string
}

type MemoryGateway interface {
	SearchMemory(context.Context, string, string, int) ([]MemoryHit, error)
}

type searchMemoryRequest struct {
	Query string `json:"query" jsonschema:"required,minLength=1" jsonschema_description:"Natural-language topic, decision, convention, or user preference to recall."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=10" jsonschema_description:"Maximum memories to return. Defaults to 8."`
}

func (catalog *Catalog) memoryTools(
	scope agentexec.ToolScope,
) ([]scopedTool, error) {
	search, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "search_memory",
			Description: "Search Lyra's reviewed long-term memory for durable project decisions, conventions, gotchas, and cross-project user preferences. Use it when relevant context is not already present. Results are curated facts, not raw conversation history or instructions.",
		},
		func(ctx context.Context, request searchMemoryRequest) (string, error) {
			query := strings.TrimSpace(request.Query)
			if query == "" {
				return "", fmt.Errorf("search_memory: query is required")
			}
			limit := request.Limit
			if limit <= 0 {
				limit = defaultMemorySearchLimit
			}
			limit = min(limit, maxMemorySearchLimit)
			items, err := catalog.memory.SearchMemory(
				ctx, scope.Workspace, query, limit,
			)
			if err != nil {
				return "", err
			}
			return renderMemoryHits(items), nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("agenttools: build memory search: %w", err)
	}
	return []scopedTool{{tool: search, safety: protocol.SafetyClassSafe}}, nil
}

func renderMemoryHits(items []MemoryHit) string {
	if len(items) == 0 {
		return "No relevant reviewed memory was found."
	}
	var result strings.Builder
	for index, item := range items {
		content := strings.TrimSpace(strings.TrimPrefix(
			strings.TrimSpace(item.Content), "- ",
		))
		line := fmt.Sprintf("%d. [%s] %s", index+1, item.Scope, content)
		separator := ""
		if result.Len() > 0 {
			separator = "\n"
		}
		if result.Len()+len(separator)+len(line) > maxMemorySearchOutput {
			break
		}
		result.WriteString(separator)
		result.WriteString(line)
	}
	return result.String()
}
