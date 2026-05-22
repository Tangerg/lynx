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
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

// App is the top-level CLI object. It owns the IO streams every
// subcommand writes to and the runtime services subcommands
// dispatch through. Construction is cheap — building the LLM
// client / engine is deferred to [App.ensureRuntime] so commands
// like `help` and `version` run without an API key.
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

	// services — built lazily on first call to ensureRuntime so
	// `lyra help` / `lyra version` don't require an API key.
	chat     chat.Service
	session  session.Service
	tool     tool.Service
	memory   memory.Service
	approval approval.Service
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

// ensureRuntime lazily builds the LLM-backed services. Idempotent —
// safe to call from every RunE entry point.
//
// Building the chat client requires a valid API key, so calling
// this from a no-args help command would falsely demand one. Help
// / version commands therefore don't call ensureRuntime; commands
// that actually talk to the model do.
func (a *App) ensureRuntime() error {
	if a.chat != nil {
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

	eng, err := engine.New(engine.Config{
		ChatClient:    client,
		Online:        config.EngineOnline(cfg),
		MCPServers:    config.EngineMCPServers(cfg),
		MemoryStore:   msgStore,
		MemoryService: memSvc,
	})
	if err != nil {
		return err
	}

	// Approval mode defaults to YOLO so the CLI path keeps the
	// "just run it" feel users had before M4. Operators that want
	// a stricter stance flip the mode at runtime via the HTTP
	// /v1/approvals/mode endpoint (or a future --approval-mode flag).
	approvalSvc := approval.New(approval.ModeYolo)

	a.chat = chat.New(eng, approvalSvc)
	a.session = sessionSvc
	a.tool = tool.New(eng)
	a.memory = memSvc
	a.approval = approvalSvc
	return nil
}

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

