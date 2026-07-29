package execution

import (
	"errors"
	"fmt"
)

// ErrInvalidRunLineage reports a Run whose root/child identity is incomplete or
// self-referential. Relationships to other Runs and Items are aggregate
// invariants checked by the tree transaction; this value owns the frame-local
// shape that every layer can validate without I/O.
var ErrInvalidRunLineage = errors.New("execution: invalid run lineage")

// RunLineage is the immutable identity edge set of a Run.
//
// A root has the zero value. A child carries all three edges: ParentRunID is the
// direct topology edge, RootRunID is the tree control/stream scope, and
// SpawnedByItemID anchors the child to the parent tool-call Item.
type RunLineage struct {
	SpawnedByItemID string
	ParentRunID     string
	RootRunID       string
}

// Validate reports whether lineage is exactly a root or child shape.
func (lineage RunLineage) Validate(runID string) error {
	if runID == "" {
		return fmt.Errorf("%w: run id is required", ErrInvalidRunLineage)
	}
	present := 0
	for _, value := range [...]string{
		lineage.SpawnedByItemID,
		lineage.ParentRunID,
		lineage.RootRunID,
	} {
		if value != "" {
			present++
		}
	}
	switch {
	case present == 0:
		return nil
	case present != 3:
		return fmt.Errorf(
			"%w: child run %q requires spawnedByItemId, parentRunId, and rootRunId together",
			ErrInvalidRunLineage,
			runID,
		)
	case lineage.ParentRunID == runID:
		return fmt.Errorf("%w: child run %q is its own parent", ErrInvalidRunLineage, runID)
	case lineage.RootRunID == runID:
		return fmt.Errorf("%w: child run %q is its own root", ErrInvalidRunLineage, runID)
	default:
		return nil
	}
}

// IsRoot reports whether lineage has the root shape.
func (lineage RunLineage) IsRoot() bool {
	return lineage == RunLineage{}
}

// IsChild reports whether lineage has the complete child shape.
func (lineage RunLineage) IsChild() bool {
	return lineage.SpawnedByItemID != "" &&
		lineage.ParentRunID != "" &&
		lineage.RootRunID != ""
}

// TreeRootID returns the root Run that owns the tree. Callers validate lineage
// before relying on the result.
func (lineage RunLineage) TreeRootID(runID string) string {
	if lineage.IsRoot() {
		return runID
	}
	return lineage.RootRunID
}
