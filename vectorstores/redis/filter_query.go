package redis

import (
	"fmt"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// buildFilterQuery turns the optional filter predicate into a
// RediSearch query string. Returns "*" (match-all) when filter is nil,
// matching the syntax FT.SEARCH expects in front of the KNN tail.
func (s *Store) buildFilterQuery(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "*", nil
	}
	v := NewVisitor(s.fieldTypes)
	if err := filter.Accept(v); err != nil {
		return "", fmt.Errorf("redis: convert filter: %w", err)
	}
	fragment := v.Result()
	if fragment == "" {
		return "*", nil
	}
	return "(" + fragment + ")", nil
}
