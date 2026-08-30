package etl_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/etl"
)

func ExampleSimpleFormatter() {
	doc, err := document.NewDocument("Scope keeps document text provider-neutral.", nil)
	if err != nil {
		panic(err)
	}
	formatted, err := etl.NewSimpleFormatter(etl.SimpleFormatterConfig{}).Format(doc)
	if err != nil {
		panic(err)
	}

	fmt.Println(formatted)
	// Output:
	// Scope keeps document text provider-neutral.
}
