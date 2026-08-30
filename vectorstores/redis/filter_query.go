package redis

import (
	"fmt"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// buildFilterQuery turns the optional filter predicate into a
// RediSearch query string. Returns "*" (match-all) when filter is nil,
// matching the syntax FT.SEARCH expects in front of the KNN tail.
func (s *Store) buildFilterQuery(expr filter.Predicate) (string, error) {
	if expr == nil {
		return "*", nil
	}
	v := newVisitor(s.fieldTypes)
	if err := expr.Accept(v); err != nil {
		return "", fmt.Errorf("redis: convert filter: %w", err)
	}
	fragment := v.snapshot()
	if fragment == "" {
		return "*", nil
	}
	return "(" + fragment + ")", nil
}
