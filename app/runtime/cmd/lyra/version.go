package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is overridden at link time via -ldflags "-X main.version=..."
// Default "dev" indicates a local / unreleased build.
var version = "dev"

// VersionCmd is `lyra version [-v]` — prints "lyra <version>" plus,
// with -v, the full debug.ReadBuildInfo dump (useful in bug
// reports — it shows which lynx-agent commit Lyra was built
// against).
func (a *App) VersionCmd() *cobra.Command {
	var verbose bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build / version info.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(a.Out, "lyra %s\n", version)
			if !verbose {
				return nil
			}
			info, ok := debug.ReadBuildInfo()
			if !ok {
				return nil
			}
			fmt.Fprintf(a.Out, "go:     %s\n", info.GoVersion)
			fmt.Fprintf(a.Out, "path:   %s\n", info.Path)
			fmt.Fprintf(a.Out, "module: %s %s\n", info.Main.Path, info.Main.Version)
			for _, dep := range info.Deps {
				if dep == nil {
					continue
				}
				fmt.Fprintf(a.Out, "  dep:  %s %s\n", dep.Path, dep.Version)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "print full build info from debug.ReadBuildInfo")
	return cmd
}
