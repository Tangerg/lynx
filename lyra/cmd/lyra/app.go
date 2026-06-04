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
	"path/filepath"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/lyra/internal/config"
	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	memorysvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	providersvc "github.com/Tangerg/lynx/lyra/internal/service/provider"
	sessionsvc "github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/internal/storage"
	sqlitestore "github.com/Tangerg/lynx/lyra/internal/storage/sqlite"
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

	sessionSvc, memSvc, procStore, interruptStore, historyStore, providerSvc, err := buildStores(cfg.Storage)
	if err != nil {
		return err
	}
	// Seed the registry with the configured provider's credentials (if not
	// already persisted from a prior providers.configure), so the default
	// provider is enabled out of the box. Other supported providers stay
	// unconfigured until the user sets their keys.
	if err := seedConfiguredProvider(ctx, providerSvc, cfg); err != nil {
		return err
	}
	// Message history stays file-backed for now (JSONL append-only
	// pattern doesn't map cleanly to SQLite rows yet).
	msgStore, err := storage.NewFileMessageStore()
	if err != nil {
		return fmt.Errorf("message storage: %w", err)
	}

	rt, err := lyraruntime.New(ctx, lyraruntime.Config{
		ChatClient: client,
		// Catalog-driven cost: price each round by its served model across
		// every provider, so turns on any provider+model report CostUSD.
		Pricing:        config.CatalogPricing(),
		Online:         cfg.Online,
		MCPServers:     cfg.MCPServers,
		MemoryStore:    msgStore,
		MemoryService:  memSvc,
		SessionService: sessionSvc,
		// Durable stores enable cross-restart HITL resume: ProcessStore
		// auto-snapshots every agent process (so a parked turn survives a
		// restart); InterruptStore persists the open-interrupt registry
		// that runs.resume looks up. Both follow LYRA_STORAGE.
		ProcessStore:    procStore,
		InterruptStore:  interruptStore,
		HistoryStore:    historyStore,
		ProviderService: providerSvc,
		// Default provider+model a turn runs against when it picks no model.
		Provider: string(cfg.Provider),
		Model:    cfg.Model,
		// ApprovalMode defaults to YOLO — operators flip the mode at
		// runtime via /v1/approvals/mode (HTTP) or a future
		// --approval-mode flag.
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

// buildStores picks the persistence backends based on the storage kind:
// session + memory + the agent-process snapshot store + the open-interrupt
// registry. SQLite shares one *sql.DB across all four at $LYRA_HOME/lyra.db;
// the file backend keeps the per-LYRA.md / sessions.json / per-process-JSON
// layout that lets users `cat` or `jq` the state directly.
//
// The process + interrupt stores are what make HITL resume survive a
// restart — they're wired the same way as session/memory so a single
// LYRA_STORAGE switch governs all durable state.
//
// The *sql.DB is intentionally leaked — process lifetime equals DB
// lifetime, and modernc.org/sqlite cleans up its WAL on close at
// exit. Add explicit teardown when the runtime grows a Shutdown
// path.
func buildStores(kind config.StorageKind) (sessionsvc.Service, memorysvc.Service, core.ProcessStore, interrupts.Store, history.Store, providersvc.Service, error) {
	switch kind {
	case config.StorageSQLite:
		home, err := storage.Home()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("sqlite storage: %w", err)
		}
		db, err := sqlitestore.Open(filepath.Join(home, "lyra.db"))
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
		return sqlitestore.NewSessionService(db), sqlitestore.NewMemoryService(db),
			sqlitestore.NewProcessStore(db), sqlitestore.NewInterruptStore(db),
			sqlitestore.NewHistoryStore(db), sqlitestore.NewProviderService(db), nil
	default:
		sess, err := storage.NewFileSessionService()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("session storage: %w", err)
		}
		mem, err := storage.NewFileMemoryService()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("memory storage: %w", err)
		}
		procStore, err := storage.NewFileProcessStore()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("process storage: %w", err)
		}
		interruptStore, err := storage.NewFileInterruptStore()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("interrupt storage: %w", err)
		}
		historyStore, err := storage.NewFileHistoryStore()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("history storage: %w", err)
		}
		providerStore, err := storage.NewFileProviderService()
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("provider storage: %w", err)
		}
		return sess, mem, procStore, interruptStore, historyStore, providerStore, nil
	}
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
var errSilenced = fmt.Errorf("lyra: handled")
