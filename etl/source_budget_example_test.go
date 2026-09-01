package etl_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/etl"
)

func ExampleSourceBudget_ReadAll() {
	budget, err := etl.NewSourceBudget(16)
	if err != nil {
		panic(err)
	}
	data, err := budget.ReadAll(context.Background(), strings.NewReader("bounded input"))
	if err != nil {
		panic(err)
	}

	fmt.Println(string(data), budget.MaxBytes())
	// Output:
	// bounded input 16
}
