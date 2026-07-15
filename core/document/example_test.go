package document_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
)

func Example() {
	doc, err := document.NewDocument("Lynx are wild cats.", nil)
	if err != nil {
		panic(err)
	}
	doc.ID = "doc-1"
	if err := metadata.Set(doc.Metadata, "source", "field-guide"); err != nil {
		panic(err)
	}
	source, _, err := metadata.Decode[string](doc.Metadata, "source")
	if err != nil {
		panic(err)
	}

	fmt.Println(doc.ID, source)
	// Output:
	// doc-1 field-guide
}
