package sqlite

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

const sessionColumns = `id, title, workspace_path, parent_id, started_at, updated_at, provider, model, favorite, isolated, revision`

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
		provider       string
		model          string
		workspacePath  string
	)
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.Title, &workspacePath, &snapshot.ParentID,
		&startedAtNanos, &updatedAtNanos, &provider, &model,
		&favoriteInt, &isolatedInt, &snapshot.Revision,
	); err != nil {
		return session.Session{}, err
	}
	snapshot.StartedAt = time.Unix(0, startedAtNanos).UTC()
	snapshot.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	snapshot.Favorite = favoriteInt != 0
	snapshot.Isolated = isolatedInt != 0
	workspace, err := session.NewWorkspace(workspacePath)
	if err != nil {
		return session.Session{}, err
	}
	snapshot.Workspace = workspace
	selection, err := modelref.New(provider, model)
	if err != nil {
		return session.Session{}, err
	}
	snapshot.Selection = selection
	value, err := session.Restore(snapshot)
	if err != nil {
		return session.Session{}, err
	}
	return value, nil
}
