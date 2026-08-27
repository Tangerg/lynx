package rag

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalidAugmentation reports empty or malformed model input produced by
// an [Augmenter].
var ErrInvalidAugmentation = errors.New("rag: invalid augmentation")

// Augmentation is the final text handed to a generation model after retrieval.
// It is intentionally distinct from [Query]: a retrieval query carries
// retrieval-scoped values, while an augmentation is generation input.
type Augmentation struct {
	text      string
	citations Citations
}

// Citation binds a one-based prompt marker to the candidate it identifies.
type Citation struct {
	Number    int       `json:"number"`
	Candidate Candidate `json:"candidate"`
}

// Citations is an ordered, one-based mapping between prompt markers and
// retrieval candidates.
type Citations []Citation

// Validate checks each citation and enforces consecutive one-based numbering.
func (c Citations) Validate() error {
	for index, citation := range c {
		if err := citation.Validate(); err != nil {
			return err
		}
		if citation.Number != index+1 {
			return fmt.Errorf(
				"%w: citation number %d at position %d, want %d",
				ErrInvalidAugmentation,
				citation.Number,
				index,
				index+1,
			)
		}
	}
	return nil
}

// NewCitation constructs and validates a citation.
func NewCitation(number int, candidate Candidate) (Citation, error) {
	citation := Citation{Number: number, Candidate: candidate}
	if err := citation.Validate(); err != nil {
		return Citation{}, err
	}
	return citation, nil
}

// Marker returns the stable prompt marker for this citation.
func (c Citation) Marker() string { return fmt.Sprintf("[%d]", c.Number) }

// Validate checks the one-based number and candidate.
func (c Citation) Validate() error {
	if c.Number <= 0 {
		return fmt.Errorf("%w: citation number must be positive", ErrInvalidAugmentation)
	}
	if err := c.Candidate.Validate(); err != nil {
		return fmt.Errorf("%w: citation %d: %w", ErrInvalidAugmentation, c.Number, err)
	}
	return nil
}

// NewAugmentation constructs validated generation input.
func NewAugmentation(text string) (Augmentation, error) {
	augmentation := Augmentation{text: text}
	if err := augmentation.Validate(); err != nil {
		return Augmentation{}, err
	}
	return augmentation, nil
}

// Text returns the final generation input.
func (a Augmentation) Text() string { return a.text }

// Citations returns an independent citation-order snapshot.
func (a Augmentation) Citations() Citations { return slices.Clone(a.citations) }

// WithCitations returns an independent augmentation with citations. Numbers
// must be consecutive and one-based so prompt markers have one interpretation.
func (a Augmentation) WithCitations(citations Citations) (Augmentation, error) {
	a.citations = slices.Clone(citations)
	if err := a.Validate(); err != nil {
		return Augmentation{}, err
	}
	return a, nil
}

// Validate checks that the generation input contains meaningful text.
func (a Augmentation) Validate() error {
	if strings.TrimSpace(a.text) == "" {
		return fmt.Errorf("%w: text must not be blank", ErrInvalidAugmentation)
	}
	return a.citations.Validate()
}

// Augmenter turns a retrieval query and its candidates into final generation
// input.
type Augmenter interface {
	Augment(ctx context.Context, query Query, candidates Candidates) (Augmentation, error)
}

// AugmenterFunc adapts a function to [Augmenter].
type AugmenterFunc func(context.Context, Query, Candidates) (Augmentation, error)

// Augment calls a(ctx, query, candidates).
func (a AugmenterFunc) Augment(ctx context.Context, query Query, candidates Candidates) (Augmentation, error) {
	return a(ctx, query, candidates)
}
