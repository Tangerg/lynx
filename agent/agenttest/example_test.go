package agenttest_test

import (
	"fmt"

	"github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/agenttest"
)

func ExampleNewMemoryTreeDurability() {
	durability := agenttest.NewMemoryTreeDurability()
	var contract agent.TreeDurability = durability

	fmt.Printf("%T\n", contract)
	// Output:
	// *agenttest.MemoryTreeDurability
}
