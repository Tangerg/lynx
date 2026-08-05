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
		s              session.Session
		startedAtNanos int64
		updatedAtNanos int64
		favoriteInt    int64
		isolatedInt    int64
	)
	if err := scanner.Scan(
		&s.ID, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &s.Model, &favoriteInt, &isolatedInt, &s.Revision,
	); err != nil {
		return session.Session{}, err
	}
	s.StartedAt = time.Unix(0, startedAtNanos).UTC()
	s.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	s.Favorite = favoriteInt != 0
	s.Isolated = isolatedInt != 0
	return s, nil
}
