// Command lyra is the terminal front end for the lyra agent runtime.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/lynx/app/cli/internal/cmd"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cmd.Execute(ctx)
}
