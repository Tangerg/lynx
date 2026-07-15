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
