package bedrockkb_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/bedrockkb"
)

func TestBuildRetrievalFilter_PreservesLargeInteger(t *testing.T) {
	result, err := bedrockkb.BuildRetrievalFilter(filter.EQ("id", uint64(math.MaxUint64)))
	if err != nil {
		t.Fatal(err)
	}
	equals, ok := result.(*types.RetrievalFilterMemberEquals)
	if !ok {
		t.Fatalf("filter = %T", result)
	}
	encoded, err := equals.Value.Value.MarshalSmithyDocument()
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "18446744073709551615" {
		t.Fatalf("encoded value = %s", encoded)
	}
}

func TestBuildRetrievalFilter_LikePreservesSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    any
	}{
		{name: "exact", pattern: "alpha", want: (*types.RetrievalFilterMemberEquals)(nil)},
		{name: "prefix", pattern: "alpha%", want: (*types.RetrievalFilterMemberStartsWith)(nil)},
		{name: "contains", pattern: "%alpha%", want: (*types.RetrievalFilterMemberStringContains)(nil)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := bedrockkb.BuildRetrievalFilter(filter.Like("name", test.pattern))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := reflect.TypeOf(result), reflect.TypeOf(test.want); got != want {
				t.Fatalf("filter type = %v, want %v", got, want)
			}
		})
	}
}

func TestBuildRetrievalFilter_RejectsLossyLikePatterns(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"%suffix", "a_b", "a%b", "%"} {
		if _, err := bedrockkb.BuildRetrievalFilter(filter.Like("name", pattern)); err == nil {
			t.Errorf("BuildRetrievalFilter(LIKE %q) error = nil, want unsupported pattern", pattern)
		}
	}
}

func TestBuildRetrievalFilter_NotIn(t *testing.T) {
	t.Parallel()

	result, err := bedrockkb.BuildRetrievalFilter(filter.Not(filter.In("status", []string{"draft", "deleted"})))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.(*types.RetrievalFilterMemberNotIn); !ok {
		t.Fatalf("filter = %T, want *types.RetrievalFilterMemberNotIn", result)
	}
}

func TestBuildRetrievalFilter_CollectionMembership(t *testing.T) {
	t.Parallel()

	result, err := bedrockkb.BuildRetrievalFilter(filter.Has("visible_to", "user-42"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.(*types.RetrievalFilterMemberListContains); !ok {
		t.Fatalf("filter = %T, want *types.RetrievalFilterMemberListContains", result)
	}
}
