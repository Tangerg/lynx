package turn

import (
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/core/media"
)

// snapshotStartTurn returns the immutable request values the asynchronous Turn
// owns.
// Runtime collaborators such as clients, observers, and callbacks keep their
// documented shared concurrency semantics and are attached later.
func snapshotStartTurn(r runs.RootExecutionStart) runs.RootExecutionStart {
	snapshot := r
	if r.Options != nil {
		options := r.Options.Clone()
		snapshot.Options = &options
	}
	if r.Media != nil {
		snapshot.Media = make([]*media.Media, len(r.Media))
		for index := range r.Media {
			snapshot.Media[index] = r.Media[index].Clone()
		}
	}
	snapshot.InterruptKinds = slices.Clone(r.InterruptKinds)
	return snapshot
}

// Handle uniquely identifies an in-flight turn. Returned by
// StartTurn and used to address subsequent operations
// (steering injection, cancellation).
type Handle struct {
	SessionID string
	TurnID    string

	// state keeps an unclaimed process-creation failure stream reachable after
	// the failed turn leaves the live registry. Reconstructed control handles
	// still resolve exclusively through TurnID.
	state *turnState
}
