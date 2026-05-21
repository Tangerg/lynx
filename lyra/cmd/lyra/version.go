package main

import (
	"flag"
	"fmt"
	"runtime/debug"
)

// version is overridden at link time via -ldflags "-X main.version=..."
// Default "dev" indicates a local / unreleased build.
var version = "dev"

// cmdVersion prints the binary version + the Go module bookkeeping
// info embedded by the toolchain. The latter is useful in bug
// reports — it shows which lynx-agent commit Lyra was built against.
func cmdVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(stderr())
	verbose := fs.Bool("v", false, "print build info from debug.ReadBuildInfo")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	fmt.Fprintf(stdout(), "lyra %s\n", version)

	if *verbose {
		if info, ok := debug.ReadBuildInfo(); ok {
			fmt.Fprintf(stdout(), "go:     %s\n", info.GoVersion)
			fmt.Fprintf(stdout(), "path:   %s\n", info.Path)
			fmt.Fprintf(stdout(), "module: %s %s\n", info.Main.Path, info.Main.Version)
			for _, dep := range info.Deps {
				if dep == nil {
					continue
				}
				fmt.Fprintf(stdout(), "  dep:  %s %s\n", dep.Path, dep.Version)
			}
		}
	}
	return 0
}
