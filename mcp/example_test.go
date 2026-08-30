package mcp_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/mcp"
)

func ExampleDiscoverTools() {
	tools, err := mcp.DiscoverTools(context.Background(), nil, mcp.ToolDiscoveryConfig{})
	if err != nil {
		panic(err)
	}

	fmt.Println(len(tools))
	// Output:
	// 0
}
