package filter_test

import (
	"fmt"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func Example() {
	built := filter.And(
		filter.EQ("category", "wildlife"),
		filter.GE("year", 2020),
	)
	parsed, err := filter.Parse(`category == 'wildlife' and year >= 2020`)
	if err != nil {
		panic(err)
	}

	fmt.Println(built.Operator(), built.Equal(parsed))
	// Output:
	// and true
}

func ExamplePredicate_String() {
	predicate := filter.And(
		filter.EQ("category", "wildlife"),
		filter.GE("year", 2020),
	)
	fmt.Println(predicate.String())
	// Output:
	// category == 'wildlife' and year >= 2020
}
