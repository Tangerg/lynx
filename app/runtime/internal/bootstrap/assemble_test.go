package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	history "github.com/Tangerg/lynx/chathistory"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/skill"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/chatclient"
)

func TestNewRequiresRuntimeDependencies(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "user home",
			edit: func(cfg *Config) {
				cfg.UserHome = ""
			},
			want: "runtime: UserHome is required",
		},
		{
			name: "relative user home",
			edit: func(cfg *Config) {
				cfg.UserHome = "relative-home"
			},
			want: "runtime: UserHome must be absolute",
		},
		{
			name: "default workspace path",
			edit: func(cfg *Config) {
				cfg.DefaultWorkspacePath = ""
			},
			want: "runtime: DefaultWorkspacePath is required",
		},
		{
			name: "relative default workspace path",
			edit: func(cfg *Config) {
				cfg.DefaultWorkspacePath = "relative-workspace"
			},
			want: "runtime: DefaultWorkspacePath must be absolute",
		},
		{
			name: "relative skills user directory",
			edit: func(cfg *Config) {
				cfg.SkillsUserDir = "relative-skills"
			},
			want: "runtime: SkillsUserDir must be absolute when set",
		},
		{
			name: "relative sandbox directory",
			edit: func(cfg *Config) {
				cfg.SandboxDir = "relative-sandbox"
			},
			want: "runtime: SandboxDir must be absolute when set",
		},
		{
			name: "relative sandbox read-only path",
			edit: func(cfg *Config) {
				cfg.SandboxReadOnlyPaths = []string{"relative-read-only"}
			},
			want: "runtime: SandboxReadOnlyPaths[0] must be absolute when set",
		},
		{
			name: "relative recipes global directory",
			edit: func(cfg *Config) {
				cfg.RecipesGlobalDir = "relative-recipes"
			},
			want: "runtime: RecipesGlobalDir must be absolute when set",
		},
		{
			name: "relative checkpoint directory",
			edit: func(cfg *Config) {
				cfg.CheckpointDir = "relative-checkpoints"
			},
			want: "runtime: CheckpointDir must be absolute when set",
		},
		{
			name: "chat client",
			edit: func(cfg *Config) {
				cfg.Engine.ChatClient = nil
			},
			want: "runtime: Engine.ChatClient is required",
		},
		{
			name: "history store",
			edit: func(cfg *Config) {
				cfg.Engine.HistoryStore = nil
			},
			want: "runtime: Engine.HistoryStore is required",
		},
		{
			name: "atomic history store",
			edit: func(cfg *Config) {
				cfg.Engine.HistoryStore = basicHistoryStore{Store: cfg.Engine.HistoryStore}
			},
			want: "runtime: Engine.HistoryStore must support atomic replace and count",
		},
		{
			name: "provider registry",
			edit: func(cfg *Config) {
				cfg.ProviderRegistry = nil
			},
			want: "runtime: ProviderRegistry is required",
		},
		{
			name: "mcp registry",
			edit: func(cfg *Config) {
				cfg.MCPRegistry = nil
			},
			want: "runtime: MCPRegistry is required",
		},
		{
			name: "mcp oauth sessions",
			edit: func(cfg *Config) {
				cfg.MCPOAuthSessions = nil
			},
			want: "runtime: MCPOAuthSessions is required",
		},
		{
			name: "session store",
			edit: func(cfg *Config) {
				cfg.SessionStore = nil
			},
			want: "runtime: SessionStore is required",
		},
		{
			name: "interrupt store",
			edit: func(cfg *Config) {
				cfg.InterruptStore = nil
			},
			want: "runtime: InterruptStore is required",
		},
		{
			name: "transcript store",
			edit: func(cfg *Config) {
				cfg.TranscriptStore = nil
			},
			want: "runtime: TranscriptStore is required",
		},
		{
			name: "run store",
			edit: func(cfg *Config) {
				cfg.RunStore = nil
			},
			want: "runtime: RunStore is required",
		},
		{
			name: "executor checkpoint store",
			edit: func(cfg *Config) {
				cfg.ExecutorCheckpoints = nil
			},
			want: "runtime: ExecutorCheckpoints is required",
		},
		{
			name: "transactor",
			edit: func(cfg *Config) {
				cfg.Transactor = nil
			},
			want: "runtime: Transactor is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			tt.edit(&cfg)

			assembly := NewAssembly(cfg)
			_, err := BuildAssembly(t.Context(), assembly)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Assembly.Build error = %v, want containing %q", err, tt.want)
			}
			_ = CloseAssembly(assembly)
		})
	}
}

// basicHistoryStore intentionally erases optional concrete capabilities so the
// composition test proves atomic replacement and counting are required.
type basicHistoryStore struct{ history.Store }

func TestAssemblyCloseBeforeBuildReleasesResourcesAndConsumesBuilder(t *testing.T) {
	var closed atomic.Int32
	assembly := NewAssembly(Config{
		Resources: []ShutdownResource{closerFunc(func() error {
			closed.Add(1)
			return nil
		})},
	})

	if err := CloseAssembly(assembly); err != nil {
		t.Fatalf("Assembly.Close: %v", err)
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("owned resource closer calls = %d, want 1", got)
	}
	if host, err := BuildAssembly(t.Context(), assembly); err == nil || host != nil {
		t.Fatalf("Build after Close = (%v, %v), want consumed Assembly", host, err)
	}
}

func TestAssemblyFailureReclaimsToolsAndOwnedResources(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	// Force engine construction to fail after the tool environment is built, so
	// the reclamation path runs; an invalid BuildID is rejected inside New.
	cfg.Engine.BuildID = "dev"
	var (
		toolClosed     atomic.Int32
		resourceClosed atomic.Int32
	)
	cfg.Resources = []ShutdownResource{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	assembly := newAssembly(cfg, func(
		ctx context.Context,
		cfg Config,
		ecfg agentexec.Config,
		policy *approval.RuntimePolicy,
		mcpEnv mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoord *scheduleapp.Coordinator,
		goalState *goals.State,
		skillStore *skillauthoring.Store,
		skillProposals skill.ProposalSubmitter,
	) (toolEnvironment, error) {
		built, err := buildToolEnvironment(ctx, cfg, ecfg, policy, mcpEnv, searcher, scheduleCoord, goalState, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		built.closers = append(built.closers, shutdown.New(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		return built, nil
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if err == nil || !strings.Contains(err.Error(), "BuildID") {
		t.Fatalf("Assembly.Build error = %v, want engine construction failure", err)
	}
	if host != nil {
		t.Fatal("failed Build returned a Host")
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
	if got := resourceClosed.Load(); got != 1 {
		t.Fatalf("owned resource closer calls = %d, want 1", got)
	}
}

func TestAssemblyBuilderFailureReclaimsReturnedAcquisitions(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	buildErr := errors.New("tool environment failed")
	var closed atomic.Int32

	assembly := newAssembly(cfg, func(
		context.Context,
		Config,
		agentexec.Config,
		*approval.RuntimePolicy,
		mcpEnvironment,
		*agentmemory.Searcher,
		*scheduleapp.Coordinator,
		*goals.State,
		*skillauthoring.Store,
		skill.ProposalSubmitter,
	) (toolEnvironment, error) {
		return toolEnvironment{
			closers: []ShutdownResource{shutdown.New(func(context.Context) error {
				closed.Add(1)
				return nil
			})},
		}, buildErr
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if !errors.Is(err, buildErr) {
		t.Fatalf("assemble error = %v, want build failure", err)
	}
	if host != nil {
		t.Fatal("successful rollback returned a Host owner")
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("returned acquisition close calls = %d, want 1", got)
	}
}

func TestAssemblyFailureRetainsRetryableCleanupOwner(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	// Fail after tools exist, then make the last tool closer fail once. The
	// Assembly must retain the same shutdown.Step so a retry continues the
	// dependency-ordered teardown instead of silently abandoning it.
	cfg.Engine.BuildID = "dev"
	closeErr := errors.New("tool close")
	var attempts, resourceClosed atomic.Int32
	cfg.Resources = []ShutdownResource{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	assembly := newAssembly(cfg, func(
		ctx context.Context,
		cfg Config,
		ecfg agentexec.Config,
		policy *approval.RuntimePolicy,
		mcpEnv mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoord *scheduleapp.Coordinator,
		goalState *goals.State,
		skillStore *skillauthoring.Store,
		skillProposals skill.ProposalSubmitter,
	) (toolEnvironment, error) {
		built, err := buildToolEnvironment(ctx, cfg, ecfg, policy, mcpEnv, searcher, scheduleCoord, goalState, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		built.closers = append(built.closers, shutdown.New(func(context.Context) error {
			if attempts.Add(1) == 1 {
				return closeErr
			}
			return nil
		}))
		return built, nil
	})
	failedHost, err := BuildAssembly(t.Context(), assembly)
	if err == nil || !strings.Contains(err.Error(), "BuildID") || !errors.Is(err, closeErr) {
		t.Fatalf("assemble error = %v, want joined engine and tool-close errors", err)
	}
	if failedHost != nil {
		t.Fatal("failed Build returned a Host")
	}
	if assembly.lifetime == nil {
		t.Fatal("failed Assembly lost ownership of incomplete tool teardown")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("tool close attempts after assemble = %d, want 1", got)
	}
	if got := resourceClosed.Load(); got != 0 {
		t.Fatalf("resource closer calls before rollback completes = %d, want 0", got)
	}
	if err := CloseAssembly(assembly); err != nil {
		t.Fatalf("retry Assembly.Close: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("tool close attempts after retry = %d, want 2", got)
	}
	if got := resourceClosed.Load(); got != 1 {
		t.Fatalf("resource closer calls after retry = %d, want 1", got)
	}
}

func TestAssemblyDirectToolsDoNotDependOnAgentResolver(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	var toolClosed atomic.Int32

	assembly := newAssembly(cfg, func(
		ctx context.Context,
		cfg Config,
		ecfg agentexec.Config,
		policy *approval.RuntimePolicy,
		mcpEnv mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoord *scheduleapp.Coordinator,
		goalState *goals.State,
		skillStore *skillauthoring.Store,
		skillProposals skill.ProposalSubmitter,
	) (toolEnvironment, error) {
		built, err := buildToolEnvironment(ctx, cfg, ecfg, policy, mcpEnv, searcher, scheduleCoord, goalState, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		built.closers = append(built.closers, shutdown.New(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		// The agent resolver is intentionally absent. Direct client-invoked
		// diagnostics have a separate fixed catalog and must not inherit the
		// agent's process-bound capability catalog.
		built.tools.Resolver = nil
		return built, nil
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("close host: %v", err)
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
}

func runtimeConfigWithRequiredDeps(t *testing.T) Config {
	t.Helper()

	client, err := chatclient.New(newReplyStub("ok"), chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	checkpoints := sqlitestore.NewExecutorCheckpointStore(db)
	mcpServers := sqlitestore.NewMCPServerStore(db)
	return Config{
		UserHome:             t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		Engine: agentexec.Config{
			ChatClient:   client,
			BuildID:      "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			HistoryStore: sqlitestore.NewMessageStore(db),
		},
		ProviderRegistry:    sqlitestore.NewProviderStore(db),
		MCPRegistry:         mcpServers,
		MCPOAuthSessions:    mcpServers,
		SessionStore:        sqlitestore.NewSessionStore(db),
		InterruptStore:      sqlitestore.NewInterruptStore(db),
		TranscriptStore:     sqlitestore.NewTranscriptStore(db),
		FeedbackStore:       sqlitestore.NewFeedbackStore(db),
		RunStore:            sqlitestore.NewRunStore(db),
		ExecutorCheckpoints: checkpoints,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
	}
}

func TestPrepareEngineConfigUsesCompositionRootPaths(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	cfg.Engine.Workdir = "/stale-engine-workdir"
	cfg.Engine.UserHome = "/stale-engine-home"

	engineConfig, _, err := prepareEngineConfig(cfg)
	if err != nil {
		t.Fatalf("prepareEngineConfig: %v", err)
	}
	if engineConfig.Workdir != cfg.DefaultWorkspacePath {
		t.Fatalf("Engine.Workdir = %q, want composition default %q", engineConfig.Workdir, cfg.DefaultWorkspacePath)
	}
	if engineConfig.UserHome != cfg.UserHome {
		t.Fatalf("Engine.UserHome = %q, want composition home %q", engineConfig.UserHome, cfg.UserHome)
	}
}

func TestAssemblyRecoversParkedRunWithIncompatibleDeployment(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		runID     = "run_park"
		sessionID = "ses_park"
		processID = "proc_park"
	)
	createdAt := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	parkedAt := createdAt.Add(time.Second)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	open := []transcript.Interrupt{{
		ItemID: "item_park", ItemOccurredAt: parkedAt,
		RunID: runID, Kind: execution.QuestionInterrupt, Question: question,
	}}

	if _, err := cfg.SessionStore.Ensure(ctx, session.Session{
		ID: sessionID, Cwd: t.TempDir(), StartedAt: createdAt, UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("ensure session: %v", err)
	}
	profile := execution.RunCapabilities{
		InterruptKinds: []execution.InterruptKind{execution.QuestionInterrupt},
	}
	if err := cfg.RunStore.Admit(ctx, execution.RunDraft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		Capabilities: profile, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.RunStore.Suspend(ctx, transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Interrupted,
		Capabilities: profile,
		Interrupts:   open, CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.TranscriptStore.AppendItem(ctx, transcript.Item{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
		Question: question, OccurredAt: parkedAt,
	}); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.InterruptStore.Open(ctx, bootstrapPending(
		runID,
		sessionID,
		processID,
		"item_park",
		createdAt,
		parkedAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	tree := bootstrapSnapshotTree(processID, core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            processID,
		Deployment:    core.DeploymentRef{Name: "chat-agent", Digest: "different-build"},
		StartedAt:     createdAt,
		Status:        core.StatusWaiting,
		Suspension: &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-park",
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     parkedAt,
		},
	})
	if err := cfg.ExecutorCheckpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(tree, sessionID, accounting.Snapshot{})); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}

	assembly := NewAssembly(cfg)
	host, err := BuildAssembly(ctx, assembly)
	if err != nil {
		t.Fatalf("Assembly.Build: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if pending, err := cfg.InterruptStore.List(ctx, sessionID); err != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, err)
	}
	if _, err := cfg.ExecutorCheckpoints.LoadCheckpoint(ctx, processID); !errors.Is(err, execution.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after assemble = %v, want not found", err)
	}
	runs, err := cfg.RunStore.ListRuns(ctx, sessionID)
	if err != nil || len(runs) != 1 || runs[0].Error == nil || runs[0].Error.Kind != transcript.RunLostProblem {
		t.Fatalf("runs after assemble = (%+v, %v), want run_lost", runs, err)
	}
}
