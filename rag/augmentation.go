package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidAugmentation reports empty or malformed model input produced by
// an [Augmenter].
var ErrInvalidAugmentation = errors.New("rag: invalid augmentation")

// Augmentation is the final text handed to a generation model after retrieval.
// It is intentionally distinct from [Query]: a retrieval query carries
// retrieval-scoped values, while an augmentation is generation input.
type Augmentation struct {
	text string
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

// Validate checks that the generation input contains meaningful text.
func (a Augmentation) Validate() error {
	if strings.TrimSpace(a.text) == "" {
		return fmt.Errorf("%w: text must not be blank", ErrInvalidAugmentation)
	}
	return nil
}

// Augmenter turns a retrieval query and its candidates into final generation
// input.
type Augmenter interface {
	Augment(ctx context.Context, query *Query, candidates []Candidate) (Augmentation, error)
}

// AugmenterFunc adapts a function to [Augmenter].
type AugmenterFunc func(context.Context, *Query, []Candidate) (Augmentation, error)

// Augment calls a(ctx, query, candidates).
func (a AugmenterFunc) Augment(ctx context.Context, query *Query, candidates []Candidate) (Augmentation, error) {
	return a(ctx, query, candidates)
}
