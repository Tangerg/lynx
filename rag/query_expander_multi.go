package rag

import (
	"context"
	"errors"
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

// MultiQueryExpanderConfig configures [NewMultiQueryExpander].
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

func (c *MultiQueryExpanderConfig) validate() error {
	if c.ChatModel == nil {
		return errors.New("rag.MultiQueryExpanderConfig: ChatModel is required")
	}
	if c.NumberOfQueries < 0 {
		return errors.New("rag.MultiQueryExpanderConfig: NumberOfQueries must be >= 0")
	}
	if c.PromptTemplate != nil {
		return c.PromptTemplate.RequireVariables("Number", "Query")
	}
	return nil
}

func (c *MultiQueryExpanderConfig) applyDefaults() {
	if c.NumberOfQueries == 0 {
		c.NumberOfQueries = defaultMultiQueryCount
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(multiExpanderDefaultTemplate)
	}
}

var _ Expander = (*multiQueryExpander)(nil)

type multiQueryExpander struct {
	chatClient      *chat.Client
	promptTemplate  *chat.PromptTemplate
	includeOriginal bool
	numberOfQueries int
}

// NewMultiQueryExpander returns an [Expander] that asks an LLM for alternate
// query phrasings.
func NewMultiQueryExpander(cfg MultiQueryExpanderConfig) (Expander, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClient(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &multiQueryExpander{
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
func (m *multiQueryExpander) Expand(ctx context.Context, query *Query) ([]*Query, error) {
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

	lines := strings.Split(expanded, "\n")
	queries := make([]*Query, 0, len(lines)+1)
	if m.includeOriginal {
		queries = append(queries, query)
	}
	limit := m.numberOfQueries
	if m.includeOriginal {
		limit++
	}

	for _, line := range lines {
		if len(queries) >= limit {
			break
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		clone, err := query.withText(text)
		if err != nil {
			return nil, err
		}
		queries = append(queries, clone)
	}

	if len(queries) == 0 {
		queries = append(queries, query)
	}
	return queries, nil
}
