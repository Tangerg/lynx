package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Tangerg/lynx/app2/runtime/cli"
)

var version = "dev"

func main() { os.Exit(run()) }

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	command := cli.New(version, os.Stdin, os.Stdout, os.Stderr)
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "lyra-runtime: %s\n", err)
		return 1
	}
	return 0
}
