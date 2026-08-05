// Command lyra is the terminal front end for the lyra agent runtime.
package main

import (
	"context"
	"os"

	"github.com/Tangerg/lynx/app/cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(context.Background()))
}
