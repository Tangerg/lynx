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
	"time"

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
	// IdempotencyNamespace identifies the exact durable replay store without
	// exposing its database path or contents.
	IdempotencyNamespace string
	Transactor           func(context.Context, func(context.Context) error) error

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

const externalChangePollInterval = 100 * time.Millisecond

// StartExternalChangeObserver reports commits made through a different SQLite
// connection. PRAGMA data_version deliberately ignores commits on this
// Bundle's own connection, whose use cases already publish precise notices.
// The baseline is read before this method returns, so a caller can expose its
// Runtime without leaving an unobserved startup window. Cancel ctx and wait on
// the returned channel to join the observer.
func (b *Bundle) StartExternalChangeObserver(ctx context.Context, notify func()) (<-chan struct{}, error) {
	if b == nil || b.db == nil || notify == nil {
		return nil, errors.New("persistence: external change observer requires a Bundle and callback")
	}
	var previous int64
	if err := b.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&previous); err != nil {
		return nil, fmt.Errorf("persistence: read external change baseline: %w", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(externalChangePollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var current int64
				if err := b.db.QueryRowContext(ctx, `PRAGMA data_version`).Scan(&current); err != nil {
					continue
				}
				if current != previous {
					previous = current
					notify()
				}
			}
		}
	}()
	return done, nil
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
	if err := os.MkdirAll(config.DataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("persistence: create data directory %q: %w", config.DataDirectory, err)
	}
	if err := os.Chmod(config.DataDirectory, 0o700); err != nil {
		return nil, fmt.Errorf("persistence: protect data directory %q: %w", config.DataDirectory, err)
	}
	db, err := sqlitestore.Open(ctx, filepath.Join(config.DataDirectory, "lyra.db"))
	if err != nil {
		return nil, err
	}
	idempotencyNamespace, err := sqlitestore.IdempotencyNamespace(ctx, db)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	knowledgeStore, err := knowledgefile.New(config.DataDirectory, config.DefaultWorkspacePath)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("knowledge storage: %w", err), db.Close())
	}
	return &Bundle{
		db:                   db,
		DataDirectory:        config.DataDirectory,
		IdempotencyNamespace: idempotencyNamespace,
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
