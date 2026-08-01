package rag

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/Tangerg/lynx/core/document"
)

// isNil recognizes both nil interfaces and interfaces containing a typed nil.
func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var (
	// ErrInvalidCandidate reports a missing/invalid document or non-finite score.
	ErrInvalidCandidate = errors.New("rag: invalid retrieval candidate")
	// ErrNilTransformer reports a missing query transformation capability.
	ErrNilTransformer = errors.New("rag: transformer must not be nil")
	// ErrNilExpander reports a missing query expansion capability.
	ErrNilExpander = errors.New("rag: expander must not be nil")
	// ErrNilRefiner reports a missing candidate refinement capability.
	ErrNilRefiner = errors.New("rag: refiner must not be nil")
	// ErrEmptyExpansion reports an expander that returned no queries.
	ErrEmptyExpansion = errors.New("rag: expander returned no queries")
)

// Candidate relates a document to the retrieval operation that produced it.
// Score is query-specific and therefore does not belong on document.Document.
type Candidate struct {
	Document *document.Document
	Score    float64
}

// Validate checks the candidate's document and score.
func (c Candidate) Validate() error {
	if c.Document == nil {
		return fmt.Errorf("%w: document must not be nil", ErrInvalidCandidate)
	}
	if err := c.Document.Validate(); err != nil {
		return fmt.Errorf("%w: document: %w", ErrInvalidCandidate, err)
	}
	if math.IsNaN(c.Score) || math.IsInf(c.Score, 0) {
		return fmt.Errorf("%w: score must be finite", ErrInvalidCandidate)
	}
	return nil
}

// Transformer rewrites a query to be more retrieval-friendly — translation,
// compression, ambiguity resolution, vocabulary normalization.
type Transformer interface {
	// Transform returns the rewritten query.
	Transform(ctx context.Context, query *Query) (*Query, error)
}

// TransformerFunc adapts a function to [Transformer].
type TransformerFunc func(context.Context, *Query) (*Query, error)

// Transform calls f(ctx, query).
func (f TransformerFunc) Transform(ctx context.Context, query *Query) (*Query, error) {
	return f(ctx, query)
}

// Expander turns one query into many — useful for poorly formed inputs
// (alternative phrasings) or complex problems (decompose into sub-queries).
type Expander interface {
	// Expand returns one or more queries derived from the input.
	Expand(ctx context.Context, query *Query) ([]*Query, error)
}

// ExpanderFunc adapts a function to [Expander].
type ExpanderFunc func(context.Context, *Query) ([]*Query, error)

// Expand calls f(ctx, query).
func (f ExpanderFunc) Expand(ctx context.Context, query *Query) ([]*Query, error) {
	return f(ctx, query)
}

// Retriever pulls candidate documents from a knowledge source.
type Retriever interface {
	// Retrieve returns documents relevant to the query.
	Retrieve(ctx context.Context, query *Query) ([]Candidate, error)
}

// RetrieverFunc adapts a function to [Retriever].
type RetrieverFunc func(context.Context, *Query) ([]Candidate, error)

// Retrieve calls f(ctx, query).
func (f RetrieverFunc) Retrieve(ctx context.Context, query *Query) ([]Candidate, error) {
	return f(ctx, query)
}

// Refiner narrows candidate documents down to what the LLM should see.
type Refiner interface {
	// Refine returns the trimmed/re-ranked document list.
	Refine(ctx context.Context, query *Query, documents []Candidate) ([]Candidate, error)
}

// RefinerFunc adapts a function to [Refiner].
type RefinerFunc func(context.Context, *Query, []Candidate) ([]Candidate, error)

// Refine calls f(ctx, query, documents).
func (f RefinerFunc) Refine(ctx context.Context, query *Query, documents []Candidate) ([]Candidate, error) {
	return f(ctx, query, documents)
}

// Augmenter folds retrieved documents into the query so the LLM has the right
// context to answer.
type Augmenter interface {
	// Augment returns a new query enriched with documents.
	Augment(ctx context.Context, query *Query, documents []Candidate) (*Query, error)
}

// AugmenterFunc adapts a function to [Augmenter].
type AugmenterFunc func(context.Context, *Query, []Candidate) (*Query, error)

// Augment calls f(ctx, query, documents).
func (f AugmenterFunc) Augment(ctx context.Context, query *Query, documents []Candidate) (*Query, error) {
	return f(ctx, query, documents)
}
