package inmemory

import (
	"math"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestEvaluatorUsesCompleteMetadataPath(t *testing.T) {
	predicate, err := filter.Parse(`profile['name'] == 'scope'`)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{
		"profile": map[string]any{"name": "scope"},
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
		{name: "pattern", source: `name like 'scope%'`},
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

func TestEvaluatorCollectionMembership(t *testing.T) {
	metadata := map[string]any{
		"tags":       []any{"go", "ai"},
		"priorities": []int{1, 2, 3},
		"scalar":     "go",
	}
	for _, test := range []struct {
		name      string
		predicate filter.Predicate
		want      bool
	}{
		{name: "present string", predicate: filter.Has("tags", "go"), want: true},
		{name: "absent string", predicate: filter.Has("tags", "rust")},
		{name: "typed numeric slice", predicate: filter.Has("priorities", 2), want: true},
		{name: "missing field", predicate: filter.Has("missing", "go")},
		{name: "scalar does not masquerade as collection", predicate: filter.Has("scalar", "go")},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := matchesFilter(test.predicate, metadata)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("matchesFilter() = %t, want %t", got, test.want)
			}
		})
	}
}
