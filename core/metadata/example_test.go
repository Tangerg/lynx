package metadata_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/metadata"
)

func Example() {
	values := metadata.New()
	if err := metadata.Set(values, "attempt", 3); err != nil {
		panic(err)
	}
	attempt, found, err := metadata.Decode[int](values, "attempt")

	fmt.Println(attempt, found, err)
	// Output:
	// 3 true <nil>
}
