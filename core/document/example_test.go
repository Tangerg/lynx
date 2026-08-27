package document_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/document"
)

func Example() {
	doc, err := document.NewDocument("Scope are wild cats.", nil)
	if err != nil {
		panic(err)
	}
	doc.ID = "doc-1"
	if setErr := doc.Metadata.Set("source", "field-guide"); setErr != nil {
		panic(setErr)
	}
	source, _, err := doc.Metadata.Decode[string]("source")
	if err != nil {
		panic(err)
	}

	fmt.Println(doc.ID, source)
	// Output:
	// doc-1 field-guide
}
