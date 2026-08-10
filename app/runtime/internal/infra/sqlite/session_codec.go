package sqlite

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

const sessionColumns = `id, title, cwd, parent_id, started_at, updated_at, model, favorite, isolated, revision`

// rowToSession decodes one DB row into a product session.Session. Execution
// continuation state deliberately lives in its dedicated sidecar table, never
// in this session projection.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		snapshot       session.Snapshot
		startedAtNanos int64
		updatedAtNanos int64
		favoriteInt    int64
		isolatedInt    int64
	)
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.Title, &snapshot.CWD, &snapshot.ParentID,
		&startedAtNanos, &updatedAtNanos, &snapshot.Model,
		&favoriteInt, &isolatedInt, &snapshot.Revision,
	); err != nil {
		return session.Session{}, err
	}
	snapshot.StartedAt = time.Unix(0, startedAtNanos).UTC()
	snapshot.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	snapshot.Favorite = favoriteInt != 0
	snapshot.Isolated = isolatedInt != 0
	value, err := session.Restore(snapshot)
	if err != nil {
		return session.Session{}, err
	}
	return value, nil
}
