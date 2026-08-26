package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
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
	// Model produces the variants. Required.
	Model chat.Model

	// IncludeOriginal prepends the original query to the variant list.
	// Defaults to false.
	IncludeOriginal bool

	// NumberOfQueries is the variant count requested from the model.
	// Defaults to [defaultMultiQueryCount]. Must be ≥ 0.
	NumberOfQueries int

	// PromptTemplate is the LLM prompt. Defaults to
	// [multiExpanderDefaultTemplate]. Custom templates must declare
	// {{.Number}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

var _ Expander = (*multiQueryExpander)(nil)

type multiQueryExpander struct {
	prompt          modelPrompt
	includeOriginal bool
	numberOfQueries int
}

type multiQueryPromptVariables struct {
	Number int
	Query  string
}

// NewMultiQueryExpander returns an [Expander] that asks an LLM for alternate
// query phrasings.
func NewMultiQueryExpander(cfg MultiQueryExpanderConfig) (Expander, error) {
	if cfg.NumberOfQueries < 0 {
		return nil, errors.New("rag: number of expanded queries must not be negative")
	}
	if cfg.NumberOfQueries == 0 {
		cfg.NumberOfQueries = defaultMultiQueryCount
	}
	prompt, err := newModelPrompt(
		cfg.Model,
		cfg.PromptTemplate,
		multiExpanderDefaultTemplate,
		promptVariableNumber,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &multiQueryExpander{
		prompt:          prompt,
		includeOriginal: cfg.IncludeOriginal,
		numberOfQueries: cfg.NumberOfQueries,
	}, nil
}

// Expand asks the LLM for variants and parses them into one [*Query]
// per non-empty line. Empty model output is reported as
// [ErrEmptyModelOutput] instead of silently turning expansion into identity.
func (m *multiQueryExpander) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	expanded, err := m.prompt.call(ctx, multiQueryPromptVariables{
		Number: m.numberOfQueries,
		Query:  query.Text(),
	})
	if err != nil {
		return nil, err
	}

	queries := make([]*Query, 0, m.numberOfQueries+1)
	if m.includeOriginal {
		queries = append(queries, query)
	}
	limit := m.numberOfQueries
	if m.includeOriginal {
		limit++
	}

	for line := range strings.SplitSeq(expanded, "\n") {
		if len(queries) >= limit {
			break
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		clone, err := query.WithText(text)
		if err != nil {
			return nil, err
		}
		queries = append(queries, clone)
	}

	if len(queries) == 0 {
		return nil, ErrEmptyExpansion
	}
	return queries, nil
}
