package goals

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// WithChangeNotices returns store wrapped so that every goal write that actually
// landed publishes its session's goal invalidation.
//
// The wrapper exists because a goal changes from many places — the lifecycle
// commands, each autonomous Run's disposition, the model's reported outcome, the
// boot reconcile — and all of them go through these three writes. Publishing at the
// write is what makes "a client is told whenever a goal moves" true by construction
// instead of by every caller remembering; it is also why the 4-second poll this
// replaces could be deleted rather than merely lengthened.
//
// A store write here is its own transaction, so returning means committed. The
// session delete/rollback cascade clears goals inside a session write-set instead,
// and that transaction publishes its own notices — one commit, one author, no
// double signal.
//
// A nil publisher returns store unchanged: nothing to notify, nothing to wrap.
func WithChangeNotices(store Store, changed change.Publish) Store {
	if store == nil || changed == nil {
		return store
	}
	return notifyingStore{Store: store, changed: changed}
}

type notifyingStore struct {
	Store
	changed change.Publish
}

func (s notifyingStore) Save(ctx context.Context, g goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	saved, applied, err := s.Store.Save(ctx, g, expected)
	if err == nil && applied {
		s.publish(g.SessionID)
	}
	return saved, applied, err
}

func (s notifyingStore) Clear(ctx context.Context, sessionID string) error {
	if err := s.Store.Clear(ctx, sessionID); err != nil {
		return err
	}
	s.publish(sessionID)
	return nil
}

func (s notifyingStore) ClearIf(ctx context.Context, sessionID string, expected goal.Version) (bool, error) {
	applied, err := s.Store.ClearIf(ctx, sessionID, expected)
	if err == nil && applied {
		s.publish(sessionID)
	}
	return applied, err
}

func (s notifyingStore) publish(sessionID string) {
	s.changed.Notify(change.InSession(change.Goals, sessionID))
}
