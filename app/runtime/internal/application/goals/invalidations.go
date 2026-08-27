package goals

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
)

// WithInvalidations returns store wrapped so that every goal write that actually
// landed publishes its session's goal invalidation.
//
// The wrapper exists because a goal changes from many places — the lifecycle
// commands, each autonomous Run's disposition, the model's reported outcome, the
// boot reconcile — and all of them go through these two writes. Publishing at the
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
func WithInvalidations(store Store, invalidations invalidation.Publish) Store {
	if store == nil || invalidations == nil {
		return store
	}
	return notifyingStore{Store: store, invalidations: invalidations}
}

type notifyingStore struct {
	Store
	invalidations invalidation.Publish
}

func (n notifyingStore) Save(ctx context.Context, g goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	saved, applied, err := n.Store.Save(ctx, g, expected)
	if err == nil && applied {
		n.publish(g.SessionID)
	}
	return saved, applied, err
}

func (n notifyingStore) ClearIf(ctx context.Context, sessionID string, expected goal.Version) (bool, error) {
	applied, err := n.Store.ClearIf(ctx, sessionID, expected)
	if err == nil && applied {
		n.publish(sessionID)
	}
	return applied, err
}

func (n notifyingStore) publish(sessionID string) {
	n.invalidations.Notify(invalidation.InSession(invalidation.Goals, sessionID))
}
