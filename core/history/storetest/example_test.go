package storetest_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/history/storetest"
)

func Example() {
	capabilities := storetest.Capabilities{Reader: true, Writer: true}
	fmt.Println(capabilities.Reader, capabilities.Writer)
	// Output: true true
}
