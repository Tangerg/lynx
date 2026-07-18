package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
			name: "chat client",
			edit: func(cfg *Config) {
				cfg.Engine.ChatClient = nil
			},
			want: "runtime: Engine.ChatClient is required",
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
			name: "process store",
			edit: func(cfg *Config) {
				cfg.ProcessStore = nil
			},
			want: "runtime: ProcessStore is required",
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

			_, err := Assemble(t.Context(), cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Assemble error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestAssembleFailureReclaimsToolsWithoutTakingCallerResources(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	cfg.Engine.SnapshotFailurePolicy = agentruntime.SnapshotFailureReportOnly
	var (
		toolClosed     atomic.Int32
		resourceClosed atomic.Int32
	)
	cfg.Resources = []io.Closer{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	_, err := assemble(t.Context(), cfg, func(
		ctx context.Context,
		cfg Config,
		ecfg agentexec.Config,
		policy approval.Policy,
		mcpEnv mcpEnvironment,
		index toolset.CodebaseIndex,
		skillStore *skillauthoring.Store,
	) (toolset.Built, error) {
		built, err := buildToolEnvironment(ctx, cfg, ecfg, policy, mcpEnv, index, skillStore)
		if err != nil {
			return toolset.Built{}, err
		}
		built.Closers = append(built.Closers, func() error {
			toolClosed.Add(1)
			return nil
		})
		return built, nil
	})
	if err == nil || !strings.Contains(err.Error(), "SnapshotFailurePolicy") {
		t.Fatalf("assemble error = %v, want engine construction failure", err)
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
	if got := resourceClosed.Load(); got != 0 {
		t.Fatalf("caller resource closer calls = %d, want 0 before successful ownership transfer", got)
	}
}

func TestAssembleFailureAfterDispatcherReclaimsTools(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	var toolClosed atomic.Int32

	_, err := assemble(t.Context(), cfg, func(
		ctx context.Context,
		cfg Config,
		ecfg agentexec.Config,
		policy approval.Policy,
		mcpEnv mcpEnvironment,
		index toolset.CodebaseIndex,
		skillStore *skillauthoring.Store,
	) (toolset.Built, error) {
		built, err := buildToolEnvironment(ctx, cfg, ecfg, policy, mcpEnv, index, skillStore)
		if err != nil {
			return toolset.Built{}, err
		}
		built.Closers = append(built.Closers, func() error {
			toolClosed.Add(1)
			return nil
		})
		// A nil catalog source fails after Agent deployment and Dispatcher
		// construction, exercising both staged cleanup guards.
		built.Resolver = nil
		return built, nil
	})
	if err == nil || !strings.Contains(err.Error(), "tool source is required") {
		t.Fatalf("assemble error = %v, want tool registry construction failure", err)
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
}

func runtimeConfigWithRequiredDeps(t *testing.T) Config {
	t.Helper()

	client, err := chatclient.New(newReplyStub("ok"))
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	db, err := sqlitestore.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	processes := sqlitestore.NewProcessStore(db)
	return Config{
		Engine: agentexec.Config{
			ChatClient: client,
			BuildID:    "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		ProviderRegistry: sqlitestore.NewProviderStore(db),
		MCPRegistry:      sqlitestore.NewMCPServerStore(db),
		SessionStore:     sqlitestore.NewSessionStore(db),
		InterruptStore:   sqlitestore.NewInterruptStore(db),
		TranscriptStore:  sqlitestore.NewTranscriptStore(db),
		RunStore:         sqlitestore.NewRunStateStore(db),
		ProcessStore:     processes,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
	}
}

func TestAssembleRecoversParkedRunWithIncompatibleDeployment(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		runID     = "run_park"
		sessionID = "ses_park"
		processID = "proc_park"
	)
	createdAt := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	parkedAt := createdAt.Add(time.Second)
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{
		ItemID: "item_park", Kind: transcript.QuestionInterrupt, Question: question,
	}}

	if err := cfg.RunStore.Admit(ctx, execution.RunDraft{RunID: runID, SessionID: sessionID, CreatedAt: createdAt}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.RunStore.Suspend(ctx, sessionID, runID); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.TranscriptStore.PutRun(ctx, transcript.Run{
		ID: runID, SessionID: sessionID, State: execution.Interrupted,
		Interrupts: open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
	}); err != nil {
		t.Fatalf("put transcript run: %v", err)
	}
	if err := cfg.TranscriptStore.AppendItem(ctx, transcript.Item{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind: transcript.QuestionItem, Status: transcript.ItemRunning,
		Question: question, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.InterruptStore.Put(ctx, interrupts.Pending{
		RunID: runID, SessionID: sessionID, TurnID: "turn_park", ProcessID: processID,
		Interrupts: open, RunCreatedAt: createdAt, CreatedAt: parkedAt,
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}
	if _, err := cfg.ProcessStore.Save(ctx, core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            processID,
		Deployment:    core.DeploymentRef{Name: "chat-agent", Digest: "different-build"},
		StartedAt:     createdAt,
		CapturedAt:    parkedAt,
		Status:        core.StatusWaiting,
		Suspension: &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-park",
			Kind:          agent.SuspensionHuman,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     parkedAt,
		},
	}, 0); err != nil {
		t.Fatalf("save process snapshot: %v", err)
	}

	host, err := Assemble(ctx, cfg)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if pending, err := cfg.InterruptStore.List(ctx, sessionID); err != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, err)
	}
	if _, err := cfg.ProcessStore.Load(ctx, processID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("process snapshot after assemble = %v, want not found", err)
	}
	_, runs, err := cfg.TranscriptStore.List(ctx, sessionID)
	if err != nil || len(runs) != 1 || runs[0].Result == nil || runs[0].Result.Error == nil ||
		runs[0].Result.Error.Kind != transcript.RunLostProblem {
		t.Fatalf("transcript after assemble = (%+v, %v), want run_lost", runs, err)
	}
}
