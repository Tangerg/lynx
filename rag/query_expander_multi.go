package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

// multiExpanderDefaultTemplate asks the LLM for N alternative phrasings.
// {{.Number}} and {{.Query}} are filled at expansion time; the native output
// contract carries the response shape.
const multiExpanderDefaultTemplate = `You are an expert at information retrieval and search optimization.
Your task is to generate {{.Number}} different versions of the given query.

Each variant must cover different perspectives or aspects of the topic,
while maintaining the core intent of the original query. The goal is to
expand the search space and improve the chances of finding relevant information.

Return exactly {{.Number}} distinct variants. Do not repeat the original query.

Original query: {{.Query}}`

const multiQueryOutputName = "rag_multi_query"

// DefaultMultiQueryCount is the variant count used when
// [MultiQueryExpanderConfig.NumberOfQueries] is unset.
const DefaultMultiQueryCount = 3

// MultiQueryExpanderConfig configures [NewMultiQueryExpander].
type MultiQueryExpanderConfig struct {
	// Model produces the variants. Required.
	Model chat.Model

	// IncludeOriginal prepends the original query to the variant list.
	// Defaults to false.
	IncludeOriginal bool

	// NumberOfQueries is the variant count requested from the model.
	// Defaults to [DefaultMultiQueryCount]. Must be ≥ 0.
	NumberOfQueries int

	// PromptTemplate is the LLM prompt. Defaults to
	// [multiExpanderDefaultTemplate]. Custom templates must declare
	// {{.Number}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

func (c MultiQueryExpanderConfig) normalized() (MultiQueryExpanderConfig, error) {
	if c.NumberOfQueries < 0 {
		return MultiQueryExpanderConfig{}, errors.New("rag: number of expanded queries must not be negative")
	}
	if c.NumberOfQueries == 0 {
		c.NumberOfQueries = DefaultMultiQueryCount
	}
	return c, nil
}

var _ Expander = (*MultiQueryExpander)(nil)

// MultiQueryExpander asks a model for alternate query phrasings.
type MultiQueryExpander struct {
	prompt          modelPrompt[multiQueryOutput]
	includeOriginal bool
	numberOfQueries int
}

type multiQueryPromptVariables struct {
	Number int
	Query  string
}

type multiQueryOutput struct {
	Queries []string `json:"queries"`
}

func (m multiQueryOutput) queries(source Query, count int, includeOriginal bool) ([]Query, error) {
	variants := make([]Query, 0, count)
	seen := map[string]struct{}{source.Text(): {}}
	for _, value := range m.Queries {
		if len(variants) >= count {
			break
		}
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if _, duplicate := seen[text]; duplicate {
			continue
		}
		seen[text] = struct{}{}
		query, err := source.WithText(text)
		if err != nil {
			return nil, err
		}
		variants = append(variants, query)
	}

	if len(variants) == 0 {
		return nil, ErrEmptyExpansion
	}
	if len(variants) != count {
		return nil, fmt.Errorf(
			"%w: model produced %d distinct variants, want %d",
			ErrInvalidExpansion,
			len(variants),
			count,
		)
	}
	if !includeOriginal {
		return variants, nil
	}
	queries := make([]Query, 0, len(variants)+1)
	queries = append(queries, source)
	return append(queries, variants...), nil
}

// NewMultiQueryExpander returns an expander that asks an LLM for alternate
// query phrasings.
func NewMultiQueryExpander(cfg MultiQueryExpanderConfig) (*MultiQueryExpander, error) {
	cfg, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	format, err := chatclient.JSONSchema[multiQueryOutput](multiQueryOutputName)
	if err != nil {
		return nil, err
	}
	prompt, err := newModelPrompt(
		cfg.Model,
		format,
		cfg.PromptTemplate,
		multiExpanderDefaultTemplate,
		promptVariableNumber,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &MultiQueryExpander{
		prompt:          prompt,
		includeOriginal: cfg.IncludeOriginal,
		numberOfQueries: cfg.NumberOfQueries,
	}, nil
}

// Expand asks the LLM for distinct variants and turns them into [Query]
// values. Empty, duplicate, and original-query entries do not consume the
// configured result limit. No usable variant returns [ErrEmptyExpansion].
func (m *MultiQueryExpander) Expand(ctx context.Context, query Query) ([]Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	output, err := m.prompt.call(ctx, multiQueryPromptVariables{
		Number: m.numberOfQueries,
		Query:  query.Text(),
	})
	if err != nil {
		if errors.Is(err, chatclient.ErrInvalidOutput) {
			return nil, fmt.Errorf("%w: model output: %w", ErrInvalidExpansion, err)
		}
		return nil, err
	}

	return output.queries(query, m.numberOfQueries, m.includeOriginal)
}
