package modeltest_test

import (
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/modeltest"
)

func Example() {
	sequence := iter.Seq2[string, error](func(yield func(string, error) bool) {
		yield("first", nil)
		yield("second", nil)
	})
	values, err := modeltest.Collect(sequence)
	fmt.Println(values, err)
	// Output: [first second] <nil>
}
