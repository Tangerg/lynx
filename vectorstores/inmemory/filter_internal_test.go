package inmemory

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestEvaluatorUsesCompleteMetadataPath(t *testing.T) {
	predicate, err := filter.Parse(`profile['name'] == 'lynx'`)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{
		"profile": map[string]any{"name": "lynx"},
	}
	matched, err := matchesFilter(predicate, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatal("nested metadata path did not match")
	}
}

func TestEvaluatorComparesLargeIntegersExactly(t *testing.T) {
	predicate := filter.EQ("sequence", uint64(math.MaxUint64))
	matched, err := matchesFilter(predicate, map[string]any{
		"sequence": uint64(math.MaxUint64 - 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Fatal("distinct uint64 values collapsed through float64")
	}
}

func TestEvaluatorTreatsMissingFieldsAsNonMatches(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "ordering", source: `rank > 10`},
		{name: "pattern", source: `name like 'lynx%'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			predicate, err := filter.Parse(tt.source)
			if err != nil {
				t.Fatal(err)
			}
			matched, err := matchesFilter(predicate, map[string]any{"other": true})
			if err != nil {
				t.Fatalf("missing field returned error: %v", err)
			}
			if matched {
				t.Fatal("missing field matched predicate")
			}
		})
	}
}
