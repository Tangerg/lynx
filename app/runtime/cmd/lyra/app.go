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

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/toolloop"
	adapterhooks "github.com/Tangerg/lynx/app/runtime/internal/adapter/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	mcpserversvc "github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	todosvc "github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
	"github.com/Tangerg/lynx/core/model/chat/history"
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
	client, err := config.BuildChatClient(cfg)
	if err != nil {
		return err
	}

	stores, err := buildStores()
	if err != nil {
		return err
	}
	// Provider registry with the stored>env credential fallback: a provider with
	// no stored key falls back to its environment variable (ANTHROPIC_API_KEY,
	// OPENAI_API_KEY, …), so a developer with keys in their shell gets those
	// providers enabled out of the box. Read once — the environment is static for
	// the process. Everything downstream (resolver, providers.list, test) goes
	// through this wrapped registry, so they share one stored>env truth.
	providers := providersvc.WithEnvKeys(stores.Provider, llm.EnvKeys())
	// Seed the registry with the configured provider's credentials (if not
	// already enabled), so the default provider works out of the box. Seeding
	// through the wrapped registry means an env-sourced default isn't redundantly
	// persisted — it stays surfaced as "from env" rather than copied to "stored".
	// Other supported providers stay unconfigured until the user sets their keys.
	if err = seedConfiguredProvider(ctx, providers, cfg); err != nil {
		return err
	}
	// Seed the config-file utility model into its store on first run, so the
	// cheaper maintenance model is honored out of the box; a persisted
	// models.setUtilityRole for the same role wins (runtime edits over config).
	if err = seedUtilityRole(ctx, stores.UtilityRole, cfg); err != nil {
		return err
	}
	// Seed env-sourced MCP servers (LYRA_MCP_SERVERS) into the registry on
	// first run; a persisted workspace.mcp.configure for the same name wins.
	if err = lyraruntime.SeedMCPServers(ctx, stores.MCPServers, cfg.MCPServers); err != nil {
		return err
	}

	// User lifecycle hooks: global hooks live at ~/.lyra/hooks.json; a project's
	// at <repo>/.lyra/hooks.json but run only once the project is trusted (a
	// cloned repo's hooks must not auto-execute). Broken hooks are recorded on
	// the turn span. userHome "" (rare) → global hooks just won't be found.
	userHome, _ := os.UserHomeDir()
	hookResolver := adapterhooks.NewResolver(userHome,
		func(projectRoot string) bool {
			ok, _ := stores.Trust.IsTrusted(context.Background(), projectRoot)
			return ok
		},
		func(hctx context.Context, source string, herr error) {
			trace.SpanFromContext(hctx).RecordError(fmt.Errorf("hook %s: %w", source, herr))
		},
	)

	rt, err := lyraruntime.New(ctx, lyraruntime.Config{
		// Engine construction config passes through verbatim (SessionStore
		// is the runtime's to fill — see runtime.Config.Engine).
		Engine: kernel.Config{
			ChatClient: client,
			// Catalog-driven cost: price each round by its served model across
			// every provider, so turns on any provider+model report CostUSD.
			Pricing: config.CatalogPricing(),
			// User-scope Agent Skills live under the storage home; per-session
			// project skills (<cwd>/.lyra/skills) layer on top of these.
			SkillsGlobalDir: filepath.Join(stores.Home, "skills"),
			HistoryStore:    stores.ChatHistory,
			Knowledge:       stores.Memory,
			// ProcessStore auto-snapshots every agent process so a parked
			// turn survives a restart (cross-restart HITL resume);
			// ParkStore persists interrupted tool rounds. Both sqlite-backed.
			ProcessStore: stores.Process,
			ParkStore:    stores.Park,
		},
		// Cheaper utility model for compaction / extraction / titling — the
		// runtime resolves it per call from this persisted role (seeded from
		// config.UtilityModel above), falling back to the main client when unset.
		UtilityRoleStore: stores.UtilityRole,
		// Tool-environment inputs — the runtime assembles the tool environment
		// (toolset.Build) from these and injects it into the engine core.
		Online:       cfg.Online,
		MCPRegistry:  stores.MCPServers,
		A2AAgents:    cfg.A2AAgents,
		LSPServers:   cfg.LSPServers, // nil means the built-in LSP server table
		SessionStore: stores.Session,
		// InterruptStore persists the open-interrupt registry that
		// runs.resume looks up — the other half of cross-restart resume.
		InterruptStore:   stores.Interrupt,
		TranscriptStore:  stores.Transcript,
		ProviderRegistry: providers,
		TodoStore:        stores.Todos,
		// Default provider+model a turn runs against when it picks no model.
		Provider: string(cfg.Provider),
		Model:    cfg.Model,
		// User lifecycle hooks (global + trusted-project), resolved per turn cwd.
		HooksResolver:  hookResolver,
		HookTrustStore: stores.Trust,
		// User-scope prompt recipes under the storage home; per-session project
		// recipes (<cwd>/.lyra/recipes) layer on top of these.
		RecipesGlobalDir: filepath.Join(stores.Home, "recipes"),
		// Scheduled runs (schedules.*) the scheduler worker fires while serving.
		ScheduleRegistry: stores.Schedules,
		// @codebase semantic index: the embedding-model role + the persisted
		// vector index (codebase_search tool + codebase.* RPC).
		EmbeddingRoleStore: stores.EmbeddingRole,
		CodebaseStore:      stores.Codebase,
		Transactor:         lyraruntime.Transactor(stores.Tx),
		// Default approval stance: Balanced — auto-allow file writes /
		// network (the agent's normal work; the user sees the diffs), prompt
		// only on shell exec, the genuinely dangerous class. Must be
		// set explicitly: approval.Mode's zero value is ModeSafe (prompts on
		// EVERY write + exec), which floods a coding session with approvals.
		// Operators flip the mode at runtime; safe/readonly/yolo are opt-in.
		ApprovalMode: approval.ModeBalanced,
		// Persistent fine-grained approval rules (AUX_API §6). nil-default in the
		// runtime treats a missing store as "no remembered rules"; production
		// always wires the sqlite-backed one.
		ApprovalRuleStore: stores.ApprovalRules,
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
// session / process-snapshot / interrupt / history / provider / chat-history
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
	mem, err := storage.NewFileKnowledgeStore()
	if err != nil {
		_ = db.Close() // close the db handle opened above on this error path
		return nil, fmt.Errorf("memory storage: %w", err)
	}
	return &Stores{
		Home: home,
		// One transaction spanning the shared *sql.DB, for the cross-store
		// write-sets (sessions.import / rollback) that must be atomic. Stores
		// route their statements through it via the context (sqlite.conn).
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
		Session:       sqlitestore.NewSessionStore(db),
		Memory:        mem,
		Process:       sqlitestore.NewProcessStore(db),
		Interrupt:     sqlitestore.NewInterruptStore(db),
		Transcript:    sqlitestore.NewTranscriptStore(db),
		Provider:      sqlitestore.NewProviderStore(db),
		MCPServers:    sqlitestore.NewMCPServerStore(db),
		ChatHistory:   sqlitestore.NewMessageStore(db),
		Park:          sqlitestore.NewParkStore(db),
		Todos:         sqlitestore.NewTodoStore(db),
		ApprovalRules: sqlitestore.NewApprovalRuleStore(db),
		UtilityRole:   sqlitestore.NewUtilityRoleStore(db),
		Trust:         sqlitestore.NewTrustStore(db),
		Schedules:     sqlitestore.NewScheduleStore(db),
		EmbeddingRole: sqlitestore.NewEmbeddingRoleStore(db),
		Codebase:      sqlitestore.NewCodebaseIndexStore(db),
	}, nil
}

// Stores bundles all persistence backends wired by [buildStores], plus the
// storage Home they share (the root for derived paths like the global skills
// directory).
type Stores struct {
	Home string
	// Tx runs fn inside one transaction across the shared sqlite *sql.DB
	// (sessions.import / rollback atomicity). Wired into the Runtime as its
	// Transactor.
	Tx            func(context.Context, func(context.Context) error) error
	Session       sessionsvc.Store
	Memory        knowledge.Store
	Process       core.ProcessStore
	Interrupt     interrupts.Store
	Transcript    transcript.Store
	Provider      providersvc.Registry
	MCPServers    mcpserversvc.Registry
	ChatHistory   history.Store
	Park          toolloop.ParkStore
	Todos         todosvc.Store
	ApprovalRules approval.RuleStore
	UtilityRole   lyraruntime.UtilityRoleStore
	Trust         *sqlitestore.TrustStore
	Schedules     *sqlitestore.ScheduleStore
	EmbeddingRole *sqlitestore.EmbeddingRoleStore
	Codebase      *sqlitestore.CodebaseIndexStore
}

// seedConfiguredProvider ensures the config-file provider is present in the
// registry with its key, so the default provider is enabled on first run. A
// provider already enabled in the registry (a persisted providers.configure)
// is left untouched — runtime edits win over the config file.
func seedConfiguredProvider(ctx context.Context, svc providersvc.Registry, cfg config.Config) error {
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

// seedUtilityRole writes the config-file utility model into the store on first
// run (when no row exists yet), pinned to the default provider. A role already
// persisted via models.setUtilityRole is left untouched — runtime edits win
// over the config file. An empty / identical-to-main UtilityModel seeds
// nothing (maintenance then runs on the main model).
func seedUtilityRole(ctx context.Context, store lyraruntime.UtilityRoleStore, cfg config.Config) error {
	if _, model, err := store.LoadUtilityRole(ctx); err != nil {
		return err
	} else if model != "" {
		return nil
	}
	if cfg.UtilityModel == "" || cfg.UtilityModel == cfg.Model {
		return nil
	}
	return store.SaveUtilityRole(ctx, string(cfg.Provider), cfg.UtilityModel)
}

// printErr writes the standard "lyra: <err>" user-facing error line to Err.
func (a *App) printErr(err error) {
	fmt.Fprintf(a.Err, "lyra: %s\n", err)
}

// fatalErr prints the error and returns a cobra-friendly silenced error so
// RunE propagates the non-zero exit code without printing a redundant
// "Error:" prefix.
func (a *App) fatalErr(err error) error {
	a.printErr(err)
	return errSilenced
}

// errSilenced is the sentinel RunE returns when the user-facing
// message has already been printed. cobra.Command.SilenceErrors +
// SilenceUsage on the root prevent the duplicate stderr.
var errSilenced = errors.New("lyra: handled")
