package execution

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalidRunTree reports a set of Run identities that cannot form one
// complete tree under the declared root.
var ErrInvalidRunTree = errors.New("execution: invalid run tree")

// RunTreeMember is the identity-only input used to assemble a RunTree. Host
// state, persistence records, executor processes, and presentation values do not
// belong here: topology is the one fact shared by all of those representations.
type RunTreeMember struct {
	RunID   string
	Lineage RunLineage
}

type runTreeInterval struct {
	start int
	end   int
}

// RunTree is an immutable, validated Run topology. Its canonical order is
// postorder: descendants before ancestors, siblings ordered lexically by Run
// ID, and the root last.
type RunTree struct {
	rootRunID string
	postorder []string
	intervals map[string]runTreeInterval
}

// NewRunTree assembles one complete tree under rootRunID. Every member must be
// present exactly once, every child must name the same root and an existing
// parent, and no disconnected component or cycle is accepted.
func NewRunTree(rootRunID string, members []RunTreeMember) (RunTree, error) {
	if strings.TrimSpace(rootRunID) == "" {
		return RunTree{}, fmt.Errorf("%w: root run id is required", ErrInvalidRunTree)
	}
	if len(members) == 0 {
		return RunTree{}, fmt.Errorf("%w: tree has no members", ErrInvalidRunTree)
	}

	lineages := make(map[string]RunLineage, len(members))
	children := make(map[string][]string, len(members))
	rootFound := false
	for index, member := range members {
		if err := member.Lineage.Validate(member.RunID); err != nil {
			return RunTree{}, fmt.Errorf(
				"%w: member[%d] run %q lineage: %w",
				ErrInvalidRunTree,
				index,
				member.RunID,
				err,
			)
		}
		if _, duplicate := lineages[member.RunID]; duplicate {
			return RunTree{}, fmt.Errorf("%w: duplicate run %q", ErrInvalidRunTree, member.RunID)
		}
		lineages[member.RunID] = member.Lineage

		if member.Lineage.IsRoot() {
			if member.RunID != rootRunID {
				return RunTree{}, fmt.Errorf(
					"%w: run %q has root lineage, want only %q",
					ErrInvalidRunTree,
					member.RunID,
					rootRunID,
				)
			}
			rootFound = true
			continue
		}
		if member.RunID == rootRunID {
			return RunTree{}, fmt.Errorf(
				"%w: root run %q carries child lineage",
				ErrInvalidRunTree,
				rootRunID,
			)
		}
		if member.Lineage.RootRunID != rootRunID {
			return RunTree{}, fmt.Errorf(
				"%w: child run %q names root %q, want %q",
				ErrInvalidRunTree,
				member.RunID,
				member.Lineage.RootRunID,
				rootRunID,
			)
		}
		children[member.Lineage.ParentRunID] = append(
			children[member.Lineage.ParentRunID],
			member.RunID,
		)
	}
	if !rootFound {
		return RunTree{}, fmt.Errorf("%w: root run %q is missing", ErrInvalidRunTree, rootRunID)
	}
	for runID, lineage := range lineages {
		if lineage.IsRoot() {
			continue
		}
		if _, exists := lineages[lineage.ParentRunID]; !exists {
			return RunTree{}, fmt.Errorf(
				"%w: child run %q names unknown parent %q",
				ErrInvalidRunTree,
				runID,
				lineage.ParentRunID,
			)
		}
	}
	for parentRunID := range children {
		slices.Sort(children[parentRunID])
	}

	tree := RunTree{
		rootRunID: rootRunID,
		postorder: make([]string, 0, len(members)),
		intervals: make(map[string]runTreeInterval, len(members)),
	}
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	states := make(map[string]uint8, len(members))
	var visit func(string) error
	visit = func(runID string) error {
		switch states[runID] {
		case visiting:
			return fmt.Errorf("%w: tree contains a cycle at run %q", ErrInvalidRunTree, runID)
		case visited:
			return nil
		}
		states[runID] = visiting
		start := len(tree.postorder)
		for _, childRunID := range children[runID] {
			if err := visit(childRunID); err != nil {
				return err
			}
		}
		tree.postorder = append(tree.postorder, runID)
		tree.intervals[runID] = runTreeInterval{start: start, end: len(tree.postorder)}
		states[runID] = visited
		return nil
	}
	if err := visit(rootRunID); err != nil {
		return RunTree{}, err
	}
	if len(tree.postorder) != len(members) {
		// With exactly one root-shaped member and every parent present, any
		// disconnected component necessarily contains a cycle. Traverse it so
		// callers receive the precise invariant violation.
		runIDs := make([]string, 0, len(lineages))
		for runID := range lineages {
			runIDs = append(runIDs, runID)
		}
		slices.Sort(runIDs)
		for _, runID := range runIDs {
			if states[runID] == unvisited {
				if err := visit(runID); err != nil {
					return RunTree{}, err
				}
			}
		}
		return RunTree{}, fmt.Errorf("%w: tree contains a Run disconnected from root %q", ErrInvalidRunTree, rootRunID)
	}
	return tree, nil
}

// RootRunID returns the tree's declared root identity.
func (tree RunTree) RootRunID() string {
	return tree.rootRunID
}

// Postorder returns a defensive copy of the complete canonical Run order.
func (tree RunTree) Postorder() []string {
	return slices.Clone(tree.postorder)
}

// SubtreePostorder returns a defensive copy of runID and all its descendants in
// canonical postorder. The boolean is false when runID is not a tree member.
func (tree RunTree) SubtreePostorder(runID string) ([]string, bool) {
	interval, exists := tree.intervals[runID]
	if !exists {
		return nil, false
	}
	return slices.Clone(tree.postorder[interval.start:interval.end]), true
}
