package filter_test

import (
	"fmt"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
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

	fmt.Println(built.Op, built.Equal(parsed))
	// Output:
	// and true
}
