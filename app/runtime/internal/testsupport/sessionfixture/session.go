// Package sessionfixture constructs valid Session aggregates for tests outside
// the session package. Production code must use real Application/Domain paths.
package sessionfixture

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// MustRestore fills irrelevant lifecycle defaults, restores snapshot, and
// panics when the requested fixture is invalid.
func MustRestore(snapshot session.Snapshot) session.Session {
	if snapshot.CWD == "" {
		snapshot.CWD = "/fixture"
	}
	if snapshot.StartedAt.IsZero() {
		snapshot.StartedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.StartedAt
	}
	if snapshot.Revision == 0 {
		snapshot.Revision = 1
	}
	if !snapshot.Selection.Configured() {
		snapshot.Selection = fixtureSelection()
	}
	value, err := session.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return value
}

func fixtureSelection() modelref.Selection {
	selection, err := modelref.New("test-provider", "test-model")
	if err != nil {
		panic(err)
	}
	return selection
}
