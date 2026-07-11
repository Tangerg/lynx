package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	sqlitestore "github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
	lyraruntime "github.com/Tangerg/lynx/app/runtime/internal/runtime"
)

// sessionForgetter releases the turn dispatcher's process-local state for a
// session being removed (the SessionStart gate). The kernel turn dispatcher
// satisfies it; the sessions coordinator's Stores surface calls it after a
// delete/purge cascade commits.
type sessionForgetter interface {
	ForgetSession(sessionID string)
}

// sessionStores is the composition root's adapter from the assembled durable
// stores to the sessions coordinator's [sessions.Stores] surface. It lets the
// coordinator depend only on what it drives — the session-scoped stores, the
// chat history log, the resume gate, and the transactional seam — instead of
// the whole runtime facade.
type sessionStores struct {
	sessions   *sqlitestore.SessionStore
	transcript *sqlitestore.TranscriptStore
	interrupts *sqlitestore.InterruptStore
	history    *conversation.Messages
	forgetter  sessionForgetter
	tx         lyraruntime.Transactor
}

func (s sessionStores) Session() sessions.SessionStore       { return s.sessions }
func (s sessionStores) Transcript() sessions.TranscriptStore { return s.transcript }
func (s sessionStores) Interrupts() sessions.InterruptStore  { return s.interrupts }

func (s sessionStores) ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error) {
	return s.history.Read(ctx, sessionID)
}

func (s sessionStores) TruncateMessages(ctx context.Context, sessionID string, keepN int) error {
	return s.history.Truncate(ctx, sessionID, keepN)
}

func (s sessionStores) SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error {
	return s.history.Seed(ctx, sessionID, msgs)
}

func (s sessionStores) ForgetSession(sessionID string) { s.forgetter.ForgetSession(sessionID) }

// RunInTx runs fn inside one storage transaction, falling back to a direct call
// when no transactor is wired (a non-sqlite / test runtime) — see
// [lyraruntime.Transactor].
func (s sessionStores) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if s.tx == nil {
		return fn(ctx)
	}
	return s.tx(ctx, fn)
}
