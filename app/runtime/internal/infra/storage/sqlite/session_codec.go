package sqlite

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

const sessionColumns = `id, user_id, agent_name, title, cwd, parent_id, started_at, updated_at, delegation_metadata, model, kind, favorite, isolated, revision`

// rowToSession decodes one DB row into a session.Session. Delegation metadata
// is an internal, opaque JSON object used only for delegated sessions.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		s              session.Session
		startedAtNanos int64
		updatedAtNanos int64
		metadataJSON   string
		kind           string
		favoriteInt    int64
		isolatedInt    int64
	)
	if err := scanner.Scan(
		&s.ID, &s.UserID, &s.AgentName, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &metadataJSON, &s.Model, &kind, &favoriteInt, &isolatedInt, &s.Revision,
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
	metadata, err := session.ParseDelegationMetadata([]byte(metadataJSON))
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: decode delegation metadata: %w", err)
	}
	s.DelegationMetadata = metadata
	return s, nil
}
