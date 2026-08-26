package run

import (
	"errors"
	"fmt"
)

// ErrInvalidLineage reports a Run whose root/child identity is incomplete or
// self-referential. Relationships to other Runs and Items are aggregate
// invariants checked by the tree transaction; this value owns the frame-local
// shape that every layer can validate without I/O.
var ErrInvalidLineage = errors.New("run: invalid lineage")

// Lineage is the immutable identity edge set of a Run.
//
// A root has the zero value. A child carries all three edges: ParentRunID is the
// direct topology edge, RootRunID is the tree control/stream scope, and
// SpawnedByItemID anchors the child to the parent tool-call Item.
type Lineage struct {
	SpawnedByItemID string
	ParentRunID     string
	RootRunID       string
}

// Validate reports whether l is exactly a root or child shape.
func (l Lineage) Validate(runID string) error {
	if runID == "" {
		return fmt.Errorf("%w: run id is required", ErrInvalidLineage)
	}
	present := 0
	for _, value := range [...]string{
		l.SpawnedByItemID,
		l.ParentRunID,
		l.RootRunID,
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
			ErrInvalidLineage,
			runID,
		)
	case l.ParentRunID == runID:
		return fmt.Errorf("%w: child run %q is its own parent", ErrInvalidLineage, runID)
	case l.RootRunID == runID:
		return fmt.Errorf("%w: child run %q is its own root", ErrInvalidLineage, runID)
	default:
		return nil
	}
}

// IsRoot reports whether l has the root shape.
func (l Lineage) IsRoot() bool {
	return l == Lineage{}
}

// IsChild reports whether l has the complete child shape.
func (l Lineage) IsChild() bool {
	return l.SpawnedByItemID != "" &&
		l.ParentRunID != "" &&
		l.RootRunID != ""
}

// TreeRootID returns the root Run that owns the tree. Callers validate l
// before relying on the result.
func (l Lineage) TreeRootID(runID string) string {
	if l.IsRoot() {
		return runID
	}
	return l.RootRunID
}
