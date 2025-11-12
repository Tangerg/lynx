package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/model/chat"
)

// MultiQueryExpanderConfig holds the configuration for MultiQueryExpander.
type MultiQueryExpanderConfig struct {
	// ChatModel is the language model used for query expansion.
	// Required.
	ChatModel chat.Model

	// IncludeOriginal determines whether to include the original query in the results.
	// Optional. Defaults to false.
	IncludeOriginal bool

	// NumberOfQueries specifies how many query variants to generate.
	// Optional. Defaults to 3 if not provided. Must be positive.
	NumberOfQueries int

	// PromptTemplate defines how the expansion prompt is structured.
	// Optional. If not provided, a default template will be used that generates
	// semantically diverse query variants covering different perspectives.
	PromptTemplate *chat.PromptTemplate
}

func (c *MultiQueryExpanderConfig) validate() error {
	if c == nil {
		return errors.New("multi expander config cannot be nil")
	}

	if c.ChatModel == nil {
		return errors.New("multi expander config: chat model is required")
	}

	if c.NumberOfQueries < 0 {
		return errors.New("multi expander config: number of queries must be positive")
	}

	if c.NumberOfQueries == 0 {
		c.NumberOfQueries = 3
	}

	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`You are an expert at information retrieval and search optimization.
Your task is to generate {{.Number}} different versions of the given query.

Each variant must cover different perspectives or aspects of the topic,
while maintaining the core intent of the original query. The goal is to
expand the search space and improve the chances of finding relevant information.

Do not explain your choices or add any other text.
Provide the query variants separated by newlines.

Original query: {{.Query}}

Query variants:`,
			)
	}

	return c.PromptTemplate.RequireVariables("Number", "Query")
}

var _ QueryExpander = (*MultiQueryExpander)(nil)

// MultiQueryExpander uses a large language model to expand a query into multiple semantically
// diverse variations to capture different perspectives, useful for retrieving additional
// contextual information and increasing the chances of finding relevant results.
//
// This expander is particularly useful when:
//   - The original query might be too narrow or specific
//   - You want to explore different angles of the same topic
//   - You need to improve recall by generating alternative phrasings
//   - You want to cover edge cases that might not match the original query
type MultiQueryExpander struct {
	chatClient      *chat.Client
	promptTemplate  *chat.PromptTemplate
	includeOriginal bool
	numberOfQueries int
}

func NewMultiQueryExpander(cfg *MultiQueryExpanderConfig) (*MultiQueryExpander, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClientWithModel(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &MultiQueryExpander{
		chatClient:      client,
		promptTemplate:  cfg.PromptTemplate,
		includeOriginal: cfg.IncludeOriginal,
		numberOfQueries: cfg.NumberOfQueries,
	}, nil
}

func (m *MultiQueryExpander) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	expandedText, _, err := m.
		chatClient.
		ChatPromptTemplate(
			m.promptTemplate.
				Clone().
				WithVariable("Number", m.numberOfQueries).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	if expandedText == "" {
		return []*Query{query}, nil
	}

	variantTexts := strings.Split(expandedText, "\n")
	queries := make([]*Query, 0, len(variantTexts)+1)

	if m.includeOriginal {
		queries = append(queries, query)
	}

	variantTexts = lo.Filter(variantTexts, func(item string, _ int) bool {
		return item != ""
	})

	for i, variantText := range variantTexts {
		if i >= m.numberOfQueries {
			break
		}
		clonedQuery := query.Clone()
		clonedQuery.Text = variantText
		queries = append(queries, clonedQuery)
	}

	if len(queries) == 0 {
		queries = append(queries, query)
	}

	return queries, nil
}
