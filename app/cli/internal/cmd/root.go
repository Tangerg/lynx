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
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/settings"
	"github.com/Tangerg/lynx/app/cli/internal/sideload"
	terminalui "github.com/Tangerg/lynx/app/cli/internal/terminal"
)

// version is overridden at link time via -ldflags "-X ...cmd.version=...".
var version = "dev"

// Version returns the client build identity advertised to runtime discovery.
func Version() string { return version }

const configIndependentAnnotation = "lyra/config-independent"

// Dependencies are the outer implementations available to the command tree.
// Runtime construction stays lazy so help and completion do not open sockets,
// databases, or other process-owned resources.
type Dependencies struct {
	OpenRuntime   func(context.Context) (agent.Runtime, error)
	RuntimeNotice string
}

// runtimeProvider delays construction until a command needs the runtime. It
// owns delivery-only diagnostics so factories remain independent of Cobra.
type runtimeProvider struct {
	open   func(context.Context) (agent.Runtime, error)
	notice string
}

func (p runtimeProvider) Open(cmd *cobra.Command) (agent.Runtime, error) {
	runtime, err := p.resolve(cmd.Context())
	if err != nil {
		return nil, err
	}
	if p.notice != "" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), p.notice); err != nil {
			return nil, fmt.Errorf("announce runtime: %w", err)
		}
	}
	return runtime, nil
}

func (p runtimeProvider) OpenQuietly(cmd *cobra.Command) (agent.Runtime, error) {
	return p.resolve(cmd.Context())
}

func (p runtimeProvider) resolve(ctx context.Context) (agent.Runtime, error) {
	if p.open == nil {
		return nil, errors.New("runtime factory is required")
	}
	return p.open(ctx)
}

// NewRoot builds an isolated command tree from process-owned dependencies.
func NewRoot(dependencies Dependencies) *cobra.Command {
	provider := runtimeProvider{
		open:   dependencies.OpenRuntime,
		notice: dependencies.RuntimeNotice,
	}
	v := viper.New()
	root := &cobra.Command{
		Use:   "lyra [prompt...]",
		Short: "Terminal front end for the lyra agent runtime",
		Long: "lyra drives an agent runtime from the terminal: an interactive session by\n" +
			"default, and one-shot runs for scripts and pipelines.",
		Example: "  # Interactive\n" +
			"  lyra\n\n" +
			"  # One-shot run, output written for a person\n" +
			"  lyra run \"why is TestCacheExpiry flaky?\"\n\n" +
			"  # One-shot run, output written for a program\n" +
			"  lyra run --json \"why is TestCacheExpiry flaky?\" > result.json\n\n" +
			"  # Stream every run event as newline-delimited JSON\n" +
			"  lyra run --output-format streaming-json \"trace the flaky test\" > run.ndjson\n\n" +
			"  # Feed a file in as context\n" +
			"  cat cache_test.go | lyra run \"explain what this test is really waiting for\"\n\n" +
			"  # List sessions\n" +
			"  lyra sessions ls",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Annotations[configIndependentAnnotation] == "true" {
				return nil
			}
			return loadConfig(v, cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := readSettings(v)
			if err != nil {
				return err
			}
			return runInteractive(cmd, args, provider, config)
		},
	}
	configureRoot(v, root)
	root.Flags().StringP("session", "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP("cwd", "C", "", "Workspace directory for a new session (default: current directory)")
	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "setup", Title: "Setup:"},
	)
	run := newRunCommand(provider, v)
	run.GroupID = "work"
	sessions := newSessionsCommand(provider)
	sessions.GroupID = "manage"
	approvals := newApprovalsCommand(provider)
	approvals.GroupID = "manage"
	config := newConfigCommand(v)
	config.GroupID = "setup"
	completion := newCompletionCommand(root)
	completion.GroupID = "setup"
	root.AddCommand(run, sessions, approvals, config, completion)
	return root
}

// runInteractive opens the terminal interface, seeding the field with whatever was typed
// on the command line.
//
// With no terminal to take over it says so and points at the command that does not
// need one, rather than failing with something about file descriptors: a program whose
// output is being piped wants text, not frames.
func runInteractive(cmd *cobra.Command, args []string, provider runtimeProvider, config settings.Config) error {
	rt, err := provider.Open(cmd)
	if err != nil {
		return err
	}
	workspacePath, err := resolveWorkspace(cmd)
	if err != nil {
		return err
	}
	// Named for the flag rather than for the package it is handed to, so the package
	// stays reachable by its own name.
	sessionID, _ := cmd.Flags().GetString("session")

	err = terminalui.Run(cmd.Context(), terminalui.Config{
		Runtime:       rt,
		SessionID:     sessionID,
		Workspace:     workspacePath,
		InitialPrompt: strings.TrimSpace(strings.Join(args, " ")),
		Settings:      new(config),
		PluginSources: []extensions.Source{sideload.New(config.Plugins.Directories)},
	})
	if errors.Is(err, term.ErrNotTerminal) {
		return errors.New("no terminal to draw on; use `lyra run` for a one-shot run")
	}
	return err
}

// resolveWorkspace resolves the directory a session works in.
func resolveWorkspace(cmd *cobra.Command) (string, error) {
	cwd, _ := cmd.Flags().GetString("cwd")
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	abs = filepath.Clean(abs)
	canonical, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return canonical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	return abs, nil
}

var errNoPrompt = errors.New("no prompt: pass one as an argument, pipe one in, or both")
