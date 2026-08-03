package storetest_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/storetest"
)

func Example() {
	capabilities := storetest.Capabilities{Indexer: true, Searcher: true}
	fmt.Println(capabilities.Indexer, capabilities.Searcher)
	// Output: true true
}
