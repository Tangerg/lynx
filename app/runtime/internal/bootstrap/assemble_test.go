package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/builtin"
	"github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/approvals"
	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
	planapp "github.com/Tangerg/lynx/app/runtime/internal/application/plans"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/teardown"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
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
				cfg.ChatClient = nil
			},
			want: "runtime: ChatClient is required",
		},
		{
			name: "conversation store",
			edit: func(cfg *Config) {
				cfg.ConversationStore = nil
			},
			want: "runtime: ConversationStore is required",
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

			assembly := NewAssembly(t.Context(), cfg)
			_, err := BuildAssembly(t.Context(), assembly)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Assembly.Build error = %v, want containing %q", err, tt.want)
			}
			_ = CloseAssembly(assembly)
		})
	}
}

func TestAssemblyCloseBeforeBuildReleasesResourcesAndConsumesBuilder(t *testing.T) {
	var closed atomic.Int32
	assembly := NewAssembly(t.Context(), Config{
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
	cfg.BuildID = "dev"
	var (
		toolClosed     atomic.Int32
		resourceClosed atomic.Int32
	)
	cfg.Resources = []ShutdownResource{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	assembly := newAssembly(t.Context(), cfg, func(
		ctx context.Context,
		lifetime context.Context,
		cfg Config,
		policy *approvals.RuntimePolicy,
		mcpConnectionSettings mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoordinator *scheduleapp.Coordinator,
		goalReader *goals.Reader,
		goalReporter *goals.OutcomeReporter,
		planCoordinator *planapp.Coordinator,
		skillStore *skillauthoring.Store,
		skillProposals builtin.SkillProposalSubmitter,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, lifetime, cfg, policy, mcpConnectionSettings, searcher, scheduleCoordinator, goalReader, goalReporter, planCoordinator, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.New(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		return toolRuntime, nil
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if err == nil || !strings.Contains(err.Error(), "build ID") {
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

	assembly := newAssembly(t.Context(), cfg, func(
		context.Context,
		context.Context,
		Config,
		*approvals.RuntimePolicy,
		mcpEnvironment,
		*agentmemory.Searcher,
		*scheduleapp.Coordinator,
		*goals.Reader,
		*goals.OutcomeReporter,
		*planapp.Coordinator,
		*skillauthoring.Store,
		builtin.SkillProposalSubmitter,
	) (toolEnvironment, error) {
		return toolEnvironment{
			closers: []ShutdownResource{teardown.New(func(context.Context) error {
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
	// Assembly must retain the same shutdown step so a retry continues the
	// dependency-ordered teardown instead of silently abandoning it.
	cfg.BuildID = "dev"
	closeErr := errors.New("tool close")
	var attempts, resourceClosed atomic.Int32
	cfg.Resources = []ShutdownResource{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	assembly := newAssembly(t.Context(), cfg, func(
		ctx context.Context,
		lifetime context.Context,
		cfg Config,
		policy *approvals.RuntimePolicy,
		mcpConnectionSettings mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoordinator *scheduleapp.Coordinator,
		goalReader *goals.Reader,
		goalReporter *goals.OutcomeReporter,
		planCoordinator *planapp.Coordinator,
		skillStore *skillauthoring.Store,
		skillProposals builtin.SkillProposalSubmitter,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, lifetime, cfg, policy, mcpConnectionSettings, searcher, scheduleCoordinator, goalReader, goalReporter, planCoordinator, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.New(func(context.Context) error {
			if attempts.Add(1) == 1 {
				return closeErr
			}
			return nil
		}))
		return toolRuntime, nil
	})
	failedHost, err := BuildAssembly(t.Context(), assembly)
	if err == nil || !strings.Contains(err.Error(), "build ID") || !errors.Is(err, closeErr) {
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

	assembly := newAssembly(t.Context(), cfg, func(
		ctx context.Context,
		lifetime context.Context,
		cfg Config,
		policy *approvals.RuntimePolicy,
		mcpConnectionSettings mcpEnvironment,
		searcher *agentmemory.Searcher,
		scheduleCoordinator *scheduleapp.Coordinator,
		goalReader *goals.Reader,
		goalReporter *goals.OutcomeReporter,
		planCoordinator *planapp.Coordinator,
		skillStore *skillauthoring.Store,
		skillProposals builtin.SkillProposalSubmitter,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, lifetime, cfg, policy, mcpConnectionSettings, searcher, scheduleCoordinator, goalReader, goalReporter, planCoordinator, skillStore, skillProposals)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.New(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		// The agent resolver is intentionally absent. Direct client-invoked
		// diagnostics have a separate fixed catalog and must not inherit the
		// model-driven Run's capability catalog.
		toolRuntime.tools.Resolver = nil
		return toolRuntime, nil
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

	checkpoints := persistence.NewExecutorCheckpointStore(sqlitestore.NewExecutorCheckpointStore(db))
	mcpServers := sqlitestore.NewMCPServerStore(db)
	return Config{
		UserHome:             t.TempDir(),
		KnowledgeDirectory:   t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		ChatClient:           client,
		BuildID:              "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ConversationStore:    sqlitestore.NewMessageStore(db),
		ProviderRegistry:     sqlitestore.NewProviderStore(db),
		MCPRegistry:          mcpServers,
		MCPOAuthSessions:     mcpServers,
		SessionStore:         sqlitestore.NewSessionStore(db),
		InterruptStore:       persistence.NewInterruptStore(sqlitestore.NewInterruptStore(db)),
		TranscriptStore:      sqlitestore.NewTranscriptStore(db),
		FeedbackStore:        sqlitestore.NewFeedbackStore(db),
		RunStore:             sqlitestore.NewRunStore(db),
		ModelInvocationStore: sqlitestore.NewModelInvocationStore(db),
		ToolInvocationStore:  sqlitestore.NewToolInvocationStore(db),
		ChildRunStartStore:   sqlitestore.NewChildRunStartReservationStore(db),
		ExecutorCheckpoints:  checkpoints,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
	}
}

func TestAssemblyRecoversParkedRunWithIncompatibleDeployment(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		runID     = "run_park"
		sessionID = "ses_park"
		memberID  = "member_park"
	)
	createdAt := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	parkedAt := createdAt.Add(time.Second)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	value := sessionfixture.MustRestore(session.Snapshot{
		ID: sessionID, CWD: t.TempDir(), StartedAt: createdAt, UpdatedAt: createdAt,
	})
	if err := cfg.SessionStore.Insert(ctx, value); err != nil {
		t.Fatalf("insert Session: %v", err)
	}
	profile := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	if err := cfg.RunStore.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		Capabilities: profile, CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.RunStore.Suspend(ctx, runfixture.MustRestore(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Waiting,
		Capabilities: profile,
		CreatedAt:    createdAt, MessageMark: run.UnknownMessageMark}),
	); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.TranscriptStore.AppendItem(ctx, itemfixture.MustRestore(itemfixture.Input{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: parkedAt,
	})); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.InterruptStore.Open(ctx, bootstrapPending(
		runID,
		sessionID,
		memberID,
		"item_park",
		createdAt,
		parkedAt,
		parkedAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	if err := cfg.ExecutorCheckpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, sessionID)); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}

	assembly := NewAssembly(t.Context(), cfg)
	host, err := BuildAssembly(ctx, assembly)
	if err != nil {
		t.Fatalf("Assembly.Build: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if pending, err := cfg.InterruptStore.List(ctx, sessionID); err != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, err)
	}
	if _, err := cfg.ExecutorCheckpoints.LoadCheckpoint(ctx, memberID); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after assemble = %v, want not found", err)
	}
	runs, err := cfg.RunStore.ListRuns(ctx, sessionID)
	failure, failed := run.Failure{}, false
	if len(runs) == 1 {
		failure, failed = runs[0].Failure()
	}
	if err != nil || len(runs) != 1 || !failed || failure.Kind != run.FailureLost {
		t.Fatalf("runs after assemble = (%+v, %v), want run_lost", runs, err)
	}
}
