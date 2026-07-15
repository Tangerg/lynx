package vectorstore_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func Example() {
	expr := filter.EQ("category", "wildlife")
	request := vectorstore.SearchRequest{
		Query:    "lynx habitat",
		TopK:     5,
		MinScore: 0.7,
		Filter:   expr,
	}
	if err := request.Validate(); err != nil {
		panic(err)
	}

	fmt.Println(request.Query, request.TopK, request.MinScore, expr.Op)
	// Output:
	// lynx habitat 5 0.7 ==
}
