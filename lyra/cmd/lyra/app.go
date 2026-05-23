// Package main implements the Lyra command-line interface — a
// cobra-based CLI on top of the in-process Service interfaces
// exposed by lyra/internal. The entrypoint constructs an [App],
// hands it the OS IO streams, and runs the cobra root command.
//
// The whole CLI is built around [App] rather than package-level
// state: every subcommand is a method on App so the runtime, the
// IO streams, and any future cross-command state live in one
// place and are trivially swappable in tests.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Tangerg/lynx/lyra/internal/config"
	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

// App is the top-level CLI object. It owns the IO streams every
// subcommand writes to and holds the runtime bundle every command
// that actually talks to the model dispatches through. Construction
// is cheap — building the LLM client / engine is deferred to
// [App.ensureRuntime] so commands like `help` and `version` run
// without an API key.
//
// Concurrency: methods are not safe for concurrent invocation —
// the CLI is single-threaded by design. Each test instance gets
// its own [App].
type App struct {
	// IO streams. Tests swap these with bytes.Buffer / strings.Reader
	// to capture and feed input.
	Out io.Writer
	Err io.Writer
	In  io.Reader

	// rt is the core runtime — built lazily on first call to
	// ensureRuntime so `lyra help` / `lyra version` don't require
	// an API key. Nil until ensureRuntime succeeds.
	rt *lyraruntime.Runtime
}

// NewApp returns an App wired to the OS standard streams. Tests
// build an App literal directly with their own streams.
func NewApp() *App {
	return &App{
		Out: os.Stdout,
		Err: os.Stderr,
		In:  os.Stdin,
	}
}

// Run executes the cobra root and returns the OS exit code. Errors
// returned by RunE on any subcommand surface via SilenceErrors;
// each subcommand renders its own user-facing message before
// returning.
func (a *App) Run(ctx context.Context, args []string) int {
	root := a.RootCmd()
	root.SetOut(a.Out)
	root.SetErr(a.Err)
	root.SetIn(a.In)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		// Subcommands print their own diagnostics — we exit non-zero
		// without doubling the message.
		return 1
	}
	return 0
}

// ensureRuntime lazily builds the runtime bundle. Idempotent —
// safe to call from every RunE entry point.
//
// Building the chat client requires a valid API key, so calling
// this from a no-args help command would falsely demand one. Help
// / version commands therefore don't call ensureRuntime; commands
// that actually talk to the model do.
func (a *App) ensureRuntime() error {
	if a.rt != nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client, err := config.BuildChatClient(cfg)
	if err != nil {
		return err
	}

	sessionSvc, err := storage.NewFileSessionService()
	if err != nil {
		return fmt.Errorf("session storage: %w", err)
	}
	msgStore, err := storage.NewFileMessageStore()
	if err != nil {
		return fmt.Errorf("message storage: %w", err)
	}
	memSvc, err := storage.NewFileMemoryService()
	if err != nil {
		return fmt.Errorf("memory storage: %w", err)
	}

	rt, err := lyraruntime.New(lyraruntime.Config{
		ChatClient:     client,
		Online:         config.EngineOnline(cfg),
		MCPServers:     config.EngineMCPServers(cfg),
		MemoryStore:    msgStore,
		MemoryService:  memSvc,
		SessionService: sessionSvc,
		// ApprovalMode defaults to YOLO — operators flip the mode at
		// runtime via /v1/approvals/mode (HTTP) or a future
		// --approval-mode flag.
	})
	if err != nil {
		return err
	}
	a.rt = rt
	return nil
}

// runtime returns the runtime bundle; subcommands call this after
// ensureRuntime succeeded. Centralises the nil-check that would
// otherwise sprinkle across every cmd file.
func (a *App) runtime() *lyraruntime.Runtime { return a.rt }

// fatalErr writes "lyra: <err>" to Err and returns a cobra-friendly
// error so RunE propagates the non-zero exit code without printing
// a redundant "Error:" prefix.
func (a *App) fatalErr(err error) error {
	fmt.Fprintf(a.Err, "lyra: %s\n", err)
	return errSilenced
}

// errSilenced is the sentinel RunE returns when the user-facing
// message has already been printed. cobra.Command.SilenceErrors +
// SilenceUsage on the root prevent the duplicate stderr.
var errSilenced = fmt.Errorf("lyra: handled")
