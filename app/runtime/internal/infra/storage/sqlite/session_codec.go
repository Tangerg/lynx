package sqlite

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

const sessionColumns = `id, user_id, agent_name, title, cwd, parent_id, started_at, updated_at, metadata, model, kind, favorite`

// rowToSession decodes one DB row into a session.Session. metadata is stored as
// a JSON blob; an empty / NULL value maps to a nil map.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		s              session.Session
		startedAtNanos int64
		updatedAtNanos int64
		metaJSON       string
		favoriteInt    int64
	)
	if err := scanner.Scan(
		&s.ID, &s.UserID, &s.AgentName, &s.Title, &s.Cwd, &s.ParentID,
		&startedAtNanos, &updatedAtNanos, &metaJSON, &s.Model, &s.Kind, &favoriteInt,
	); err != nil {
		return session.Session{}, err
	}
	s.StartedAt = time.Unix(0, startedAtNanos).UTC()
	s.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	s.Favorite = favoriteInt != 0
	if metaJSON != "" && metaJSON != "{}" {
		if err := json.Unmarshal([]byte(metaJSON), &s.Metadata); err != nil {
			return session.Session{}, fmt.Errorf("sqlite: decode metadata: %w", err)
		}
	}
	return s, nil
}

// encodeMetadata marshals the metadata map; nil / empty maps become "{}" so
// the row's NOT NULL constraint stays satisfied.
func encodeMetadata(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("sqlite: encode metadata: %w", err)
	}
	return string(data), nil
}
