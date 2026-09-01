package rag_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/rag"
)

func ExampleTopK() {
	first, err := document.NewDocument("first", nil)
	if err != nil {
		panic(err)
	}
	second, err := document.NewDocument("second", nil)
	if err != nil {
		panic(err)
	}
	query, err := rag.NewQuery("rank the evidence")
	if err != nil {
		panic(err)
	}
	refiner, err := rag.TopK(1)
	if err != nil {
		panic(err)
	}
	result, err := refiner.Refine(context.Background(), query, rag.Candidates{
		{Document: first, Score: 0.25},
		{Document: second, Score: 0.75},
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(result[0].Document.Text)
	// Output:
	// second
}
