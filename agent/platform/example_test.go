package platform_test

import (
	"fmt"

	"github.com/Tangerg/scope/agent/platform"
)

func ExampleNewCatalog() {
	catalog, err := platform.NewCatalog()
	if err != nil {
		panic(err)
	}

	fmt.Println(len(catalog.Deployments()))
	// Output:
	// 0
}
