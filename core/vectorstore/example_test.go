package vectorstore_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func Example() {
	expr := filter.EQ("category", "wildlife")
	request := &vectorstore.SearchRequest{
		Query: "scope habitat",
		Options: vectorstore.SearchOptions{
			TopK: 5, MinScore: 0.7, Filter: expr,
		},
	}
	if err := request.Validate(); err != nil {
		panic(err)
	}

	fmt.Println(request.Query, request.Options.TopK, request.Options.MinScore, expr.Operator())
	// Output:
	// scope habitat 5 0.7 ==
}
