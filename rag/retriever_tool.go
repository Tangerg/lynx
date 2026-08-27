package rag

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/tool"
	"github.com/samber/lo"
)

// RetrievalToolConfig describes a model-visible retrieval capability. Name
// and Description follow [chat.ToolDefinition] requirements. Retriever is
// required and may already be composed with transformers, expansion, fusion,
// and refiners.
type RetrievalToolConfig struct {
	Name        string
	Description string
	Retriever   Retriever
}

// RetrievalToolRequest is the strict model-generated tool input.
type RetrievalToolRequest struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Natural-language query to retrieve evidence for."`
}

// RetrievalToolOutput is the model-visible retrieval result.
type RetrievalToolOutput struct {
	Candidates Candidates `json:"candidates"`
}

// Validate checks every returned candidate.
func (o RetrievalToolOutput) Validate() error { return o.Candidates.Validate() }

// RetrievalTool adapts a [Retriever] to the ordinary [tool.Tool] contract. It
// can be advertised immediately or placed in an agent's DeferredTools set.
type RetrievalTool struct {
	function tool.Func[RetrievalToolRequest, RetrievalToolOutput]
}

var _ tool.Tool = RetrievalTool{}

// NewRetrievalTool constructs a strictly decoded, schema-derived retrieval
// tool without adding an agent dependency to this package.
func NewRetrievalTool(config RetrievalToolConfig) (RetrievalTool, error) {
	if lo.IsNil(config.Retriever) {
		return RetrievalTool{}, ErrNilRetriever
	}
	function, err := tool.NewFunc(
		tool.FuncConfig{Name: config.Name, Description: config.Description},
		func(ctx context.Context, input RetrievalToolRequest) (RetrievalToolOutput, error) {
			query, err := NewQuery(input.Query)
			if err != nil {
				return RetrievalToolOutput{}, fmt.Errorf("rag: retrieval tool query: %w", err)
			}
			candidates, err := Retrieve(ctx, config.Retriever, query)
			if err != nil {
				return RetrievalToolOutput{}, err
			}
			return RetrievalToolOutput{Candidates: candidates}, nil
		},
	)
	if err != nil {
		return RetrievalTool{}, err
	}
	return RetrievalTool{function: function}, nil
}

// Definition returns an independent model-visible tool definition.
func (r RetrievalTool) Definition() chat.ToolDefinition { return r.function.Definition() }

// Call validates arguments, retrieves candidates, and returns their JSON
// representation.
func (r RetrievalTool) Call(ctx context.Context, arguments string) (string, error) {
	return r.function.Call(ctx, arguments)
}
