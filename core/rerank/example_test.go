package rerank_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/rerank"
)

func Example() {
	request, err := rerank.NewRequest("capital of France", []string{
		"Berlin is the capital of Germany.",
		"Paris is the capital of France.",
	})
	if err != nil {
		panic(err)
	}
	request.Options.Model = "provider-reranker"

	fmt.Println(request.Query, len(request.Documents))
	// Output:
	// capital of France 2
}
