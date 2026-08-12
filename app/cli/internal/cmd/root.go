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
	"github.com/Tangerg/lynx/app/cli/internal/backend"
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
	OpenRuntime    func(context.Context) (backend.Services, error)
	RuntimeNotice  string
	StateDirectory string
}

// runtimeProvider delays construction until a command needs the runtime. It
// owns delivery-only diagnostics so factories remain independent of Cobra.
type runtimeProvider struct {
	open   func(context.Context) (backend.Services, error)
	notice string
}

func (p runtimeProvider) Open(cmd *cobra.Command) (agent.Runtime, error) {
	services, err := p.OpenServices(cmd)
	if err != nil {
		return nil, err
	}
	return services.Agent, nil
}

func (p runtimeProvider) OpenServices(cmd *cobra.Command) (backend.Services, error) {
	services, err := p.resolve(cmd.Context())
	if err != nil {
		return backend.Services{}, err
	}
	if p.notice != "" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), p.notice); err != nil {
			return backend.Services{}, fmt.Errorf("announce runtime: %w", err)
		}
	}
	return services, nil
}

func (p runtimeProvider) OpenQuietly(cmd *cobra.Command) (agent.Runtime, error) {
	services, err := p.resolve(cmd.Context())
	return services.Agent, err
}

func (p runtimeProvider) resolve(ctx context.Context) (backend.Services, error) {
	if p.open == nil {
		return backend.Services{}, errors.New("runtime factory is required")
	}
	services, err := p.open(ctx)
	if err != nil {
		return backend.Services{}, err
	}
	if err := services.Validate(); err != nil {
		return backend.Services{}, err
	}
	return services, nil
}

// NewRoot builds an isolated command tree from process-owned dependencies.
func NewRoot(dependencies Dependencies) *cobra.Command {
	provider := runtimeProvider{
		open:   dependencies.OpenRuntime,
		notice: dependencies.RuntimeNotice,
	}
	v := viper.New()
	root := newRootCommand(v, provider, dependencies.StateDirectory)
	configureRoot(v, root)
	root.Flags().StringP("session", "s", "", "Open an existing session instead of a new one")
	root.PersistentFlags().StringP("cwd", "C", "", "Workspace directory for a new session (default: current directory)")
	root.AddGroup(
		&cobra.Group{ID: "work", Title: "Work:"},
		&cobra.Group{ID: "manage", Title: "Manage:"},
		&cobra.Group{ID: "setup", Title: "Setup:"},
	)
	addRootCommands(root, provider, v)
	return root
}

func newRootCommand(v *viper.Viper, provider runtimeProvider, stateDirectory string) *cobra.Command {
	return &cobra.Command{
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
			return runInteractive(cmd, args, provider, config, stateDirectory)
		},
	}
}

func addRootCommands(root *cobra.Command, provider runtimeProvider, v *viper.Viper) {
	run := newRunCommand(provider, v)
	run.GroupID = "work"
	sessions := newSessionsCommand(provider)
	sessions.GroupID = "manage"
	runs := newRunsCommand(provider)
	runs.GroupID = "manage"
	approvals := newApprovalsCommand(provider)
	approvals.GroupID = "manage"
	config := newConfigCommand(v)
	config.GroupID = "setup"
	completion := newCompletionCommand(root)
	completion.GroupID = "setup"
	root.AddCommand(run, sessions, runs, approvals, config, completion)
}

// runInteractive opens the terminal interface, seeding the field with whatever was typed
// on the command line.
//
// With no terminal to take over it says so and points at the command that does not
// need one, rather than failing with something about file descriptors: a program whose
// output is being piped wants text, not frames.
func runInteractive(cmd *cobra.Command, args []string, provider runtimeProvider, config settings.Config, stateDirectory string) error {
	services, err := provider.OpenServices(cmd)
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
		Runtime:          services.Agent,
		Workspaces:       services.Workspaces,
		Changes:          services.Changes,
		Transfers:        services.Transfers,
		Usage:            services.Usage,
		ModelConfig:      services.ModelConfig,
		Goals:            services.Goals,
		Skills:           services.Skills,
		MCP:              services.MCP,
		Schedules:        services.Schedules,
		AgentMemory:      services.AgentMemory,
		Knowledge:        services.Knowledge,
		DiagnosticTools:  services.DiagnosticTools,
		Codebase:         services.Codebase,
		AuthoringContext: services.AuthoringContext,
		Hooks:            services.Hooks,
		Feedback:         services.Feedback,
		SessionID:        sessionID,
		Workspace:        workspacePath,
		InitialPrompt:    strings.TrimSpace(strings.Join(args, " ")),
		Settings:         new(config),
		PluginSources:    []extensions.Source{sideload.New(config.Plugins.Directories)},
		StateDirectory:   stateDirectory,
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
	return canonicalWorkspacePath(cwd)
}

func canonicalWorkspacePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
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
