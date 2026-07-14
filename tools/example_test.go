package tools_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/tools"
)

func ExampleNew() {
	type input struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	add, err := tools.New(tools.Config{
		Name:        "add",
		Description: "add two integers",
	}, func(_ context.Context, value input) (int, error) {
		return value.A + value.B, nil
	})
	if err != nil {
		panic(err)
	}
	registry, err := tools.NewRegistry(add)
	if err != nil {
		panic(err)
	}

	fmt.Println(registry.Definitions()[0].Name)
	result, err := add.Call(context.Background(), `{"a":2,"b":3}`)
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
	// Output:
	// add
	// 5
}
