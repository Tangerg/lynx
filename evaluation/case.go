package evaluation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// CaseID is a stable identity within one Dataset.
type CaseID string

func (id CaseID) String() string { return string(id) }

func (id CaseID) Validate() error {
	value := id.String()
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%w: id must be non-empty without surrounding whitespace", ErrInvalidCase)
	}
	return nil
}

// Case gives a stable identity to one evaluation subject.
type Case[T any] struct {
	ID       CaseID
	Subject  T
	Metadata metadata.Map
}

func (caseValue Case[T]) Validate() error {
	if err := caseValue.ID.Validate(); err != nil {
		return err
	}
	if err := caseValue.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidCase, err)
	}
	return nil
}

func (caseValue Case[T]) clone() Case[T] {
	caseValue.Metadata = caseValue.Metadata.Clone()
	return caseValue
}

// Dataset is an immutable ordered set of uniquely identified cases. Evaluators
// must not mutate subjects; metadata is owned and cloned by the Dataset.
type Dataset[T any] struct{ cases []Case[T] }

func NewDataset[T any](cases ...Case[T]) (Dataset[T], error) {
	owned := slices.Clone(cases)
	seen := make(map[CaseID]struct{}, len(owned))
	for index, caseValue := range owned {
		if err := caseValue.Validate(); err != nil {
			return Dataset[T]{}, fmt.Errorf("%w: cases[%d]: %w", ErrInvalidDataset, index, err)
		}
		if _, exists := seen[caseValue.ID]; exists {
			return Dataset[T]{}, fmt.Errorf("%w: duplicate id %q", ErrInvalidDataset, caseValue.ID)
		}
		seen[caseValue.ID] = struct{}{}
		owned[index] = caseValue.clone()
	}
	return Dataset[T]{cases: owned}, nil
}

func (dataset Dataset[T]) Len() int { return len(dataset.cases) }

// Cases returns an owned copy in deterministic declaration order.
func (dataset Dataset[T]) Cases() []Case[T] {
	cases := slices.Clone(dataset.cases)
	for index := range cases {
		cases[index] = cases[index].clone()
	}
	return cases
}
