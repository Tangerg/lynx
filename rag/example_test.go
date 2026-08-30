package rag_test

import (
	"fmt"

	"github.com/Tangerg/scope/rag"
)

func ExampleQuery_WithValue() {
	query, err := rag.NewQuery("how does retrieval work?")
	if err != nil {
		panic(err)
	}
	tenant, err := rag.NewValueKey[string]("tenant")
	if err != nil {
		panic(err)
	}
	query, err = query.WithValue(tenant, "docs")
	if err != nil {
		panic(err)
	}
	value, found, err := query.Value(tenant)
	if err != nil {
		panic(err)
	}

	fmt.Println(query.Text(), value, found)
	// Output:
	// how does retrieval work? docs true
}
