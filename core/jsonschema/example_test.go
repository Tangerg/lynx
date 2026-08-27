package jsonschema_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/jsonschema"
)

func Example() {
	type answer struct {
		Value int `json:"value"`
	}
	schema, err := jsonschema.For[answer]()
	if err != nil {
		panic(err)
	}

	fmt.Println(schema.Validate([]byte(`{"value":42}`)))
	// Output:
	// <nil>
}
