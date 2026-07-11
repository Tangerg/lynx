// Package persistence assembles Lyra's durable storage adapters into one
// process-lifetime bundle. It is the storage-side capability adapter: the
// process composition root opens a bundle, while runtime construction decides
// how to consume it.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat/history"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	mcpserversvc "github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	providersvc "github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	sessionsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	todosvc "github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// Bundle holds every persistence backend opened for one runtime process. All
// durable stores share one SQLite database at $LYRA_HOME/lyra.db, except
// Knowledge, which is the user-editable LYRA.md cascade.
type Bundle struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error

	Home string
	Tx   func(context.Context, func(context.Context) error) error

	Session       sessionsvc.Store
	Memory        knowledge.Store
	Process       core.ProcessStore
	Interrupt     interrupts.Store
	Transcript    transcript.Store
	Provider      providersvc.Registry
	MCPServers    mcpserversvc.Registry
	ChatHistory   history.Store
	Park          agentexec.ParkStore
	Todos         todosvc.Store
	ApprovalRules approval.RuleStore
	UtilityRole   *sqlitestore.UtilityRoleStore
	Trust         *sqlitestore.TrustStore
	Schedules     *sqlitestore.ScheduleStore
	EmbeddingRole *sqlitestore.EmbeddingRoleStore
	Codebase      *sqlitestore.CodebaseIndexStore
}

// Open wires the persistence backends. The returned bundle owns the shared
// SQLite handle and must be closed when the runtime process stops.
func Open() (*Bundle, error) {
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
		return nil, errors.Join(fmt.Errorf("memory storage: %w", err), db.Close())
	}
	return &Bundle{
		db:   db,
		Home: home,
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
		Park:          agentexec.AsParkStore(sqlitestore.NewParkStore(db)),
		Todos:         sqlitestore.NewTodoStore(db),
		ApprovalRules: sqlitestore.NewApprovalRuleStore(db),
		UtilityRole:   sqlitestore.NewUtilityRoleStore(db),
		Trust:         sqlitestore.NewTrustStore(db),
		Schedules:     sqlitestore.NewScheduleStore(db),
		EmbeddingRole: sqlitestore.NewEmbeddingRoleStore(db),
		Codebase:      sqlitestore.NewCodebaseIndexStore(db),
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
