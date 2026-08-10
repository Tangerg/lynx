// Package cmd is the CLI's command tree.
//
// Commands are built by constructors rather than declared as package variables,
// so a test can build a fresh tree with its own runtime and its own output
// buffers. Flag state does not survive between trees, which is what makes the
// commands testable in-memory.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/sideload"
	terminalui "github.com/Tangerg/lynx/app/cli/internal/terminal"
)

// version is overridden at link time via -ldflags "-X ...cmd.version=...".
var version = "dev"

// mockNotice tells the user, on stderr so stdout stays pipeable, that nothing
// here talks to a real agent yet.
const mockNotice = "lyra: scripted mock runtime — no backend is wired in yet"

// backend delays runtime construction until a command actually needs it.
// Completion has a separate path because it must stay silent: diagnostics on
// stderr are useful during execution but become shell-completion noise.
type backend struct {
	open       func(*cobra.Command) (client.Runtime, error)
	completion func(*cobra.Command) (client.Runtime, error)
}

func (b backend) forCompletion(cmd *cobra.Command) (client.Runtime, error) {
	if b.completion != nil {
		return b.completion(cmd)
	}
	return b.open(cmd)
}

// Execute runs the CLI and reports the process exit code.
func Execute(ctx context.Context) int {
	if err := NewRoot(nil).ExecuteContext(ctx); err != nil {
		// Cobra has already printed the error.
		return 1
	}
	return 0
}

// NewRoot builds the command tree. A nil runtime installs the scripted mock,
// which is what a real build does today; tests pass their own.
func NewRoot(rt client.Runtime) *cobra.Command {
	resolve := func(*cobra.Command) (client.Runtime, error) {
		if rt != nil {
			return rt, nil
		}
		return mock.New(), nil
	}
	return newRootWithBackend(backend{
		open: func(cmd *cobra.Command) (client.Runtime, error) {
			if rt == nil {
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), mockNotice); err != nil {
					return nil, fmt.Errorf("announce mock runtime: %w", err)
				}
			}
			return resolve(cmd)
		},
		completion: resolve,
	})
}

// newRootWithBackend builds the tree around a way of getting a runtime, which is
// the seam the real backends will arrive through.
func newRootWithBackend(resolve backend) *cobra.Command {
	v := viper.New()
	root := &cobra.Command{
		Use:   "lyra",
		Short: "Terminal front end for the lyra agent runtime",
		Long: "lyra drives an agent runtime from the terminal: an interactive session by\n" +
			"default, and one-shot runs for scripts and pipelines.",
		Example: "  # Interactive\n" +
			"  lyra\n\n" +
			"  # One-shot run, output written for a person\n" +
			"  lyra run \"why is TestCacheExpiry flaky?\"\n\n" +
			"  # One-shot run, output written for a program\n" +
			"  lyra run --json \"why is TestCacheExpiry flaky?\" > run.ndjson\n\n" +
			"  # Feed a file in as context\n" +
			"  cat cache_test.go | lyra run \"explain what this test is really waiting for\"\n\n" +
			"  # List sessions\n" +
			"  lyra sessions ls",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return loadConfig(v, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readSettings(v)
			if err != nil {
				return err
			}
			return interactive(cmd, args, resolve, value)
		},
	}
	configure(v, root)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.Flags().StringP("session", "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP("cwd", "C", "", "Workspace directory for a new session (default: current directory)")
	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "setup", Title: "Setup:"},
	)
	run := newRunCommand(resolve, v)
	run.GroupID = "work"
	sessions := newSessionsCommand(resolve)
	sessions.GroupID = "manage"
	approvals := newApprovalsCommand(resolve)
	approvals.GroupID = "manage"
	config := newConfigCommand(v)
	config.GroupID = "setup"
	completion := newCompletionCommand(root)
	completion.GroupID = "setup"
	root.AddCommand(run, sessions, approvals, config, completion)
	return root
}

// interactive opens the terminal interface, seeding the field with whatever was typed
// on the command line.
//
// With no terminal to take over it says so and points at the command that does not
// need one, rather than failing with something about file descriptors: a program whose
// output is being piped wants text, not frames.
func interactive(cmd *cobra.Command, args []string, resolve backend, value settings.Settings) error {
	rt, err := resolve.open(cmd)
	if err != nil {
		return err
	}
	ws, err := workspace(cmd)
	if err != nil {
		return err
	}
	// Named for the flag rather than for the package it is handed to, so the package
	// stays reachable by its own name.
	sessionID, _ := cmd.Flags().GetString("session")

	err = terminalui.Run(cmd.Context(), terminalui.Config{
		Runtime:       rt,
		Session:       sessionID,
		Workspace:     ws,
		Prompt:        strings.TrimSpace(strings.Join(args, " ")),
		Settings:      value,
		PluginSources: []extensions.Source{sideload.New(value.Plugins.Directories)},
	})
	if errors.Is(err, term.ErrNotTerminal) {
		return errors.New("no terminal to draw on; use `lyra run` for a one-shot run")
	}
	return err
}

// workspace resolves the directory a session works in.
func workspace(cmd *cobra.Command) (string, error) {
	if cwd, _ := cmd.Flags().GetString("cwd"); cwd != "" {
		return cwd, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return cwd, nil
}

// sessionFor finds the session a command should run in: the one named, or a new
// one in this workspace.
func sessionFor(ctx context.Context, rt interface {
	client.SessionReader
	client.SessionWriter
}, id, ws string) (client.Session, error) {
	if id == "" {
		return rt.CreateSession(ctx, client.NewSession{Workspace: ws})
	}
	snapshot, err := rt.GetSession(ctx, id)
	if err != nil {
		return client.Session{}, fmt.Errorf("open session: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return client.Session{}, fmt.Errorf("open session: %w", err)
	}
	return snapshot.Session, nil
}

var errNoPrompt = errors.New("no prompt: pass one as an argument, pipe one in, or both")
