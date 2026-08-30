package fs_test

import (
	"fmt"

	toolfs "github.com/Tangerg/scope/tools/fs"
)

func ExampleNewReadTool() {
	read := toolfs.NewReadTool(nil)
	definition := read.Definition()

	fmt.Println(definition.Name)
	// Output:
	// read
}
