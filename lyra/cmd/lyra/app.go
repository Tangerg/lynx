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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	chatmem "github.com/Tangerg/lynx/core/model/chat/middleware/memory"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/infra/storage"
	sqlitestore "github.com/Tangerg/lynx/lyra/internal/infra/storage/sqlite"
	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
	"github.com/Tangerg/lynx/lyra/internal/service/approval"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/service/knowledge"
	providersvc "github.com/Tangerg/lynx/lyra/internal/service/provider"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/service/session"
	todosvc "github.com/Tangerg/lynx/lyra/internal/service/todo"
	"github.com/Tangerg/lynx/lyra/internal/service/transcript"
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

	// cfg is the config loaded on the first ensureRuntime; serve reads
	// its Server section (listen / cors / token gate) from here.
	cfg config.Config
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
func (a *App) ensureRuntime(ctx context.Context) error {
	if a.rt != nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// The default client is the configured provider+model — the one a turn
	// runs against when it doesn't pick a model. Per-run model selection
	// builds other clients on demand from the provider registry.
	client, _, err := config.BuildChatClient(cfg)
	if err != nil {
		return err
	}

	// Optional cheaper model for turn-boundary maintenance (compaction /
	// extraction / planning), on the same provider + credentials as the main
	// client — only the model id differs. Unset or identical → maintenance
	// runs on the main client (nil MaintenanceClient).
	var maintClient *chat.Client
	if cfg.MaintenanceModel != "" && cfg.MaintenanceModel != cfg.Model {
		maintClient, _, err = config.BuildClient(config.ClientSpec{
			Provider: cfg.Provider,
			Model:    cfg.MaintenanceModel,
			APIKey:   cfg.APIKey,
			BaseURL:  cfg.BaseURL,
		})
		if err != nil {
			return fmt.Errorf("build maintenance client: %w", err)
		}
	}

	stores, err := buildStores()
	if err != nil {
		return err
	}
	// Seed the registry with the configured provider's credentials (if not
	// already persisted from a prior providers.configure), so the default
	// provider is enabled out of the box. Other supported providers stay
	// unconfigured until the user sets their keys.
	if err = seedConfiguredProvider(ctx, stores.Provider, cfg); err != nil {
		return err
	}

	rt, err := lyraruntime.New(ctx, lyraruntime.Config{
		// Engine construction config passes through verbatim (SessionStore
		// is the runtime's to fill — see runtime.Config.Engine).
		Engine: engine.Config{
			ChatClient: client,
			// Catalog-driven cost: price each round by its served model across
			// every provider, so turns on any provider+model report CostUSD.
			Pricing: config.CatalogPricing(),
			// User-scope Agent Skills live under the storage home; per-session
			// project skills (<cwd>/.lyra/skills) layer on top of these.
			SkillsGlobalDir: filepath.Join(stores.Home, "skills"),
			MemoryStore:     stores.ChatMem,
			Knowledge:       stores.Memory,
			// ProcessStore auto-snapshots every agent process so a parked
			// turn survives a restart (cross-restart HITL resume);
			// ParkStore persists interrupted tool rounds. Both sqlite-backed.
			ProcessStore: stores.Process,
			ParkStore:    stores.Park,
		},
		// Cheaper maintenance model (nil → maintenance runs on the main client).
		MaintenanceClient: maintClient,
		// Tool-environment inputs — the runtime assembles the tool environment
		// (toolset.Build) from these and injects it into the engine core.
		Online:         cfg.Online,
		MCPServers:     cfg.MCPServers,
		A2AAgents:      cfg.A2AAgents,
		LSPServers:     cfg.LSPServers, // nil → toolset uses codeintel.DefaultServers()
		SessionService: stores.Session,
		// InterruptStore persists the open-interrupt registry that
		// runs.resume looks up — the other half of cross-restart resume.
		InterruptStore:  stores.Interrupt,
		TranscriptStore: stores.History,
		ProviderService: stores.Provider,
		TodoService:     stores.Todos,
		// Default provider+model a turn runs against when it picks no model.
		Provider: string(cfg.Provider),
		Model:    cfg.Model,
		// Default approval stance: Balanced — auto-allow file writes /
		// network (the agent's normal work; the user sees the diffs), prompt
		// only on shell exec (bash), the genuinely dangerous class. Must be
		// set explicitly: approval.Mode's zero value is ModeSafe (prompts on
		// EVERY write + exec), which floods a coding session with approvals.
		// Operators flip the mode at runtime; safe/readonly/yolo are opt-in.
		ApprovalMode: approval.ModeBalanced,
	})
	if err != nil {
		return err
	}
	a.rt = rt
	a.cfg = cfg
	return nil
}

// runtime returns the runtime bundle; subcommands call this after
// ensureRuntime succeeded. Centralizes the nil-check that would
// otherwise sprinkle across every cmd file.
func (a *App) runtime() *lyraruntime.Runtime { return a.rt }

// config returns the loaded config; valid after ensureRuntime.
func (a *App) config() config.Config { return a.cfg }

// buildStores wires the persistence backends. Everything durable —
// session / process-snapshot / interrupt / history / provider / chat-memory
// messages — shares one SQLite *sql.DB at $LYRA_HOME/lyra.db. The one
// exception is the LYRA.md memory cascade: it stays a user-editable file
// (the whole point of it is that the user can `cat` / edit it), so it
// doesn't live in SQLite.
//
// The process + interrupt stores are what make HITL resume survive a
// restart. The *sql.DB is intentionally process-lifetime (no teardown);
// modernc.org/sqlite cleans up its WAL on exit. Add explicit teardown when
// the runtime grows a Shutdown path.
func buildStores() (*Stores, error) {
	home, err := storage.Home()
	if err != nil {
		return nil, fmt.Errorf("storage home: %w", err)
	}
	db, err := sqlitestore.Open(filepath.Join(home, "lyra.db"))
	if err != nil {
		return nil, err
	}
	mem, err := storage.NewFileKnowledgeService()
	if err != nil {
		_ = db.Close() // we opened it; don't leak the handle on the error path
		return nil, fmt.Errorf("memory storage: %w", err)
	}
	return &Stores{
		Home:      home,
		Session:   sqlitestore.NewSessionService(db),
		Memory:    mem,
		Process:   sqlitestore.NewProcessStore(db),
		Interrupt: sqlitestore.NewInterruptStore(db),
		History:   sqlitestore.NewTranscriptStore(db),
		Provider:  sqlitestore.NewProviderService(db),
		ChatMem:   sqlitestore.NewMessageStore(db),
		Park:      sqlitestore.NewParkStore(db),
		Todos:     sqlitestore.NewTodoService(db),
	}, nil
}

// Stores bundles all persistence backends wired by [buildStores], plus the
// storage Home they share (the root for derived paths like the global skills
// directory).
type Stores struct {
	Home      string
	Session   sessionsvc.Service
	Memory    knowledge.Service
	Process   core.ProcessStore
	Interrupt interrupts.Store
	History   transcript.Store
	Provider  providersvc.Service
	ChatMem   chatmem.Store
	Park      tool.ParkStore
	Todos     todosvc.Service
}

// seedConfiguredProvider ensures the config-file provider is present in the
// registry with its key, so the default provider is enabled on first run. A
// provider already enabled in the registry (a persisted providers.configure)
// is left untouched — runtime edits win over the config file.
func seedConfiguredProvider(ctx context.Context, svc providersvc.Service, cfg config.Config) error {
	id := string(cfg.Provider)
	if existing, ok, err := svc.Get(ctx, id); err != nil {
		return err
	} else if ok && existing.Enabled() {
		return nil
	}
	return svc.Configure(ctx, providersvc.Provider{
		ID:      id,
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
	})
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
var errSilenced = errors.New("lyra: handled")
