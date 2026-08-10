// Package persistence assembles Lyra's durable storage adapters into one
// process-lifetime bundle. It is the storage-side capability adapter: [Open]
// returns a bundle while its consumers decide how to use each store.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/knowledgefile"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

// Bundle holds every persistence backend opened for one runtime process. All
// durable stores share one SQLite database under DataDirectory, except
// Knowledge, which is the user-editable LYRA.md cascade. AgentMemory is the
// separate SQLite fact ledger + curated memory items.
type Bundle struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error

	DataDirectory string
	Transactor    func(context.Context, func(context.Context) error) error

	Sessions            *sqlitestore.SessionStore
	Runs                *sqlitestore.RunStore
	WorkspaceMutations  *WorkspaceMutationStore
	Knowledge           *knowledgefile.Store
	AgentMemory         *sqlitestore.AgentMemoryStore
	ExecutorCheckpoints *ExecutorCheckpointStore
	Interrupts          *InterruptStore
	Transcript          *sqlitestore.TranscriptStore
	Feedback            *sqlitestore.FeedbackStore
	Providers           *sqlitestore.ProviderStore
	MCPServers          *sqlitestore.MCPServerStore
	ChatHistory         *sqlitestore.MessageStore
	ModelInvocations    *sqlitestore.ModelInvocationStore
	ToolInvocations     *sqlitestore.ToolInvocationStore
	ChildRunStarts      *sqlitestore.ChildRunStartReservationStore
	Plan                *sqlitestore.PlanStore
	PermissionModes     *sqlitestore.PermissionModeStore
	Goals               *sqlitestore.GoalStore
	ApprovalRules       *sqlitestore.ApprovalRuleStore
	UtilityRole         *sqlitestore.UtilityRoleStore
	Trust               *sqlitestore.TrustStore
	Schedules           *sqlitestore.ScheduleStore
	EmbeddingRole       *sqlitestore.EmbeddingRoleStore
	CodebaseIndex       *sqlitestore.CodebaseIndexStore
	ToolResults         *sqlitestore.ToolResultStore
	Idempotency         *sqlitestore.IdempotencyStore
}

// Config is the process-owned filesystem snapshot persistence consumes. It has
// no environment or working-directory fallback: startup supplies every path.
type Config struct {
	DataDirectory        string
	DefaultWorkspacePath string
}

// Open wires the persistence backends. The returned bundle owns the shared
// SQLite handle and must be closed when the runtime process stops.
func Open(ctx context.Context, config Config) (*Bundle, error) {
	if config.DataDirectory == "" {
		return nil, errors.New("persistence: data directory is required")
	}
	if !filepath.IsAbs(config.DataDirectory) {
		return nil, errors.New("persistence: data directory must be absolute")
	}
	if config.DefaultWorkspacePath == "" {
		return nil, errors.New("persistence: default workspace path is required")
	}
	if !filepath.IsAbs(config.DefaultWorkspacePath) {
		return nil, errors.New("persistence: default workspace path must be absolute")
	}
	if err := os.MkdirAll(config.DataDirectory, 0o755); err != nil {
		return nil, fmt.Errorf("persistence: create data directory %q: %w", config.DataDirectory, err)
	}
	db, err := sqlitestore.Open(ctx, filepath.Join(config.DataDirectory, "lyra.db"))
	if err != nil {
		return nil, err
	}
	knowledgeStore, err := knowledgefile.New(config.DataDirectory, config.DefaultWorkspacePath)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("knowledge storage: %w", err), db.Close())
	}
	return &Bundle{
		db:            db,
		DataDirectory: config.DataDirectory,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
		Sessions:            sqlitestore.NewSessionStore(db),
		Runs:                sqlitestore.NewRunStore(db),
		WorkspaceMutations:  NewWorkspaceMutationStore(sqlitestore.NewWorkspaceMutationStore(db)),
		Knowledge:           knowledgeStore,
		AgentMemory:         sqlitestore.NewAgentMemoryStore(db),
		ExecutorCheckpoints: NewExecutorCheckpointStore(sqlitestore.NewExecutorCheckpointStore(db)),
		Interrupts:          NewInterruptStore(sqlitestore.NewInterruptStore(db)),
		Transcript:          sqlitestore.NewTranscriptStore(db),
		Feedback:            sqlitestore.NewFeedbackStore(db),
		Providers:           sqlitestore.NewProviderStore(db),
		MCPServers:          sqlitestore.NewMCPServerStore(db),
		ChatHistory:         sqlitestore.NewMessageStore(db),
		ModelInvocations:    sqlitestore.NewModelInvocationStore(db),
		ToolInvocations:     sqlitestore.NewToolInvocationStore(db),
		ChildRunStarts:      sqlitestore.NewChildRunStartReservationStore(db),
		Plan:                sqlitestore.NewPlanStore(db),
		PermissionModes:     sqlitestore.NewPermissionModeStore(db),
		Goals:               sqlitestore.NewGoalStore(db),
		ApprovalRules:       sqlitestore.NewApprovalRuleStore(db),
		UtilityRole:         sqlitestore.NewUtilityRoleStore(db),
		Trust:               sqlitestore.NewTrustStore(db),
		Schedules:           sqlitestore.NewScheduleStore(db),
		EmbeddingRole:       sqlitestore.NewEmbeddingRoleStore(db),
		CodebaseIndex:       sqlitestore.NewCodebaseIndexStore(db),
		ToolResults:         sqlitestore.NewToolResultStore(db),
		Idempotency:         sqlitestore.NewIdempotencyStore(db),
	}, nil
}

// Close releases the shared SQLite handle. It is safe to call repeatedly.
func (b *Bundle) Close() error {
	if b == nil {
		return nil
	}
	b.closeOnce.Do(func() {
		if b.db != nil {
			b.closeErr = b.db.Close()
		}
	})
	return b.closeErr
}

// Shutdown implements the context-aware process-resource boundary.
// SQLite's Close is synchronous and normally immediate, but checking the
// deadline before beginning prevents a shutdown attempt from starting new work
// after its caller's budget has expired.
func (b *Bundle) Shutdown(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.Close()
}
