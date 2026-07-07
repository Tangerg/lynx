package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/lynx/agent/core"
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
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
	"github.com/Tangerg/lynx/core/model/chat/history"
)

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
		Park:          kernel.AsParkStore(sqlitestore.NewParkStore(db)),
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
	Park          kernel.ParkStore
	Todos         todosvc.Store
	ApprovalRules approval.RuleStore
	UtilityRole   lyraruntime.UtilityRoleStore
	Trust         *sqlitestore.TrustStore
	Schedules     *sqlitestore.ScheduleStore
	EmbeddingRole *sqlitestore.EmbeddingRoleStore
	Codebase      *sqlitestore.CodebaseIndexStore
}
