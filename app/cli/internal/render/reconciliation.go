package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

// resolveSnapshotRun selects the already accepted run from a cold projection.
// Falling back to the latest run is reserved for direct renderer use where no
// Begin call established an identity.
func resolveSnapshotRun(snapshot agent.SessionSnapshot, runID string) (agent.Run, error) {
	if strings.TrimSpace(runID) == "" {
		latest, ok := snapshot.LatestRun()
		if !ok {
			return agent.Run{}, errors.New("snapshot has no run")
		}
		return latest, nil
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return agent.Run{}, fmt.Errorf("snapshot does not contain run %s", runID)
	}
	return run, nil
}
