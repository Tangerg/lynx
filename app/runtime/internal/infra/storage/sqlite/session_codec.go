package sqlite

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

const sessionColumns = `id, title, cwd, parent_id, started_at, updated_at, model, kind, favorite, isolated, revision`

// rowToSession decodes one DB row into a product session.Session. Agent-runtime
// continuation state deliberately lives in the bootstrap-owned sidecar table,
// never in this domain projection.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		s              session.Session
		startedAtNanos int64
		updatedAtNanos int64
		kind           string
		favoriteInt    int64
		isolatedInt    int64
	)
	if err := scanner.Scan(
		&s.ID, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &s.Model, &kind, &favoriteInt, &isolatedInt, &s.Revision,
	); err != nil {
		return session.Session{}, err
	}
	s.StartedAt = time.Unix(0, startedAtNanos).UTC()
	s.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	s.Favorite = favoriteInt != 0
	s.Isolated = isolatedInt != 0
	parsedKind, err := session.ParseKind(kind)
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: decode session kind: %w", err)
	}
	s.Kind = parsedKind
	return s, nil
}
