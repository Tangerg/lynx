package bedrockkb_test

import (
	"math"
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
