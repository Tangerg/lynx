package workflow_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/scope/agent/workflow"
)

func ExampleTransform() {
	stage, err := workflow.Transform("normalize", func(input string) (string, error) {
		return strings.ToUpper(input), nil
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(stage.Valid())
	// Output:
	// true
}
