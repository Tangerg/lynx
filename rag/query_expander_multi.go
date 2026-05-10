package rag

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// multiExpanderDefaultTemplate asks the LLM for N alternative phrasings
// of the user's query, one per line, no commentary. {{.Number}} and
// {{.Query}} are filled at expansion time.
const multiExpanderDefaultTemplate = `You are an expert at information retrieval and search optimization.
Your task is to generate {{.Number}} different versions of the given query.

Each variant must cover different perspectives or aspects of the topic,
while maintaining the core intent of the original query. The goal is to
expand the search space and improve the chances of finding relevant information.

Do not explain your choices or add any other text.
Provide the query variants separated by newlines.

Original query: {{.Query}}

Query variants:`

// defaultMultiQueryCount is the variant count used when
// [MultiQueryExpanderConfig.NumberOfQueries] is unset.
const defaultMultiQueryCount = 3

// MultiQueryExpanderConfig configures a [MultiQueryExpander].
type MultiQueryExpanderConfig struct {
	// ChatModel produces the variants. Required.
	ChatModel chat.Model

	// IncludeOriginal prepends the original query to the variant list.
	// Defaults to false.
	IncludeOriginal bool

	// NumberOfQueries is the variant count requested from the model.
	// Defaults to [defaultMultiQueryCount]. Must be ≥ 0.
	NumberOfQueries int

	// PromptTemplate is the LLM prompt. Defaults to
	// [multiExpanderDefaultTemplate]. Custom templates must declare
	// {{.Number}} and {{.Query}}.
	PromptTemplate *chat.PromptTemplate
}

// validate fills defaults and rejects invalid configs.
func (c *MultiQueryExpanderConfig) validate() error {
	if c == nil {
		return errors.New("rag.MultiQueryExpanderConfig: config must not be nil")
	}
	if c.ChatModel == nil {
		return errors.New("rag.MultiQueryExpanderConfig: ChatModel is required")
	}
	if c.NumberOfQueries < 0 {
		return errors.New("rag.MultiQueryExpanderConfig: NumberOfQueries must be ≥ 0")
	}
	if c.NumberOfQueries == 0 {
		c.NumberOfQueries = defaultMultiQueryCount
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(multiExpanderDefaultTemplate)
	}
	return c.PromptTemplate.RequireVariables("Number", "Query")
}

var _ QueryExpander = (*MultiQueryExpander)(nil)

// MultiQueryExpander asks an LLM to rephrase the user's query into N
// semantically diverse variants — useful when the original query is
// narrow, ambiguous, or might miss relevant documents under different
// phrasings.
//
// Example:
//
//	exp, err := rag.NewMultiQueryExpander(&rag.MultiQueryExpanderConfig{
//	    ChatModel: model, NumberOfQueries: 5,
//	})
//	queries, err := exp.Expand(ctx, q)
type MultiQueryExpander struct {
	chatClient      *chat.Client
	promptTemplate  *chat.PromptTemplate
	includeOriginal bool
	numberOfQueries int
}

// NewMultiQueryExpander builds a [MultiQueryExpander]. Returns an error
// when the configuration fails validation or the chat client cannot be
// constructed.
func NewMultiQueryExpander(cfg *MultiQueryExpanderConfig) (*MultiQueryExpander, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClient(cfg.ChatModel)
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

// Expand asks the LLM for variants and parses them into one [*Query]
// per non-empty line. When the model returns nothing usable the
// original query is returned, ensuring downstream retrieval always
// has at least one query to run.
func (m *MultiQueryExpander) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	expanded, _, err := m.chatClient.
		ChatWithPromptTemplate(
			m.promptTemplate.Clone().
				WithVariable("Number", m.numberOfQueries).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	if expanded == "" {
		return []*Query{query}, nil
	}

	variants := slices.DeleteFunc(strings.Split(expanded, "\n"), func(s string) bool {
		return s == ""
	})

	queries := make([]*Query, 0, len(variants)+1)
	if m.includeOriginal {
		queries = append(queries, query)
	}

	for i, text := range variants {
		if i >= m.numberOfQueries {
			break
		}
		clone := query.Clone()
		clone.Text = text
		queries = append(queries, clone)
	}

	if len(queries) == 0 {
		queries = append(queries, query)
	}
	return queries, nil
}
