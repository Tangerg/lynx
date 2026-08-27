package main

import (
	"context"
	"fmt"
	"os"
)

const executableName = "scopeapp"

// version is overridden at link time via -ldflags "-X main.version=...".
// Default "dev" indicates a local / unreleased build.
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "%s: unexpected argument %q; this binary only starts the HTTP runtime server\n", executableName, os.Args[1])
		os.Exit(2)
	}
	if err := run(context.Background(), os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", executableName, err)
		os.Exit(1)
	}
}
