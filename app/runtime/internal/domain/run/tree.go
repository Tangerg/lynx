package run

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalidTree reports a set of Run identities that cannot form one
// complete tree under the declared root.
var ErrInvalidTree = errors.New("run: invalid tree")

// TreeMember is the identity-only input used to assemble a Tree. Host
// state, persistence records, executor state, and presentation values do not
// belong here: topology is the one fact shared by all of those representations.
type TreeMember struct {
	RunID   string
	Lineage Lineage
}

type runTreeInterval struct {
	start int
	end   int
}

const (
	runTreeUnvisited uint8 = iota
	runTreeVisiting
	runTreeVisited
)

type runTreeBuilder struct {
	rootRunID string
	lineages  map[string]Lineage
	children  map[string][]string
	states    map[string]uint8
	tree      Tree
}

// Tree is an immutable, validated Run topology. Its canonical order is
// postorder: descendants before ancestors, siblings ordered lexically by Run
// ID, and the root last.
type Tree struct {
	rootRunID string
	postorder []string
	intervals map[string]runTreeInterval
}

// NewTree assembles one complete tree under rootRunID. Every member must be
// present exactly once, every child must name the same root and an existing
// parent, and no disconnected component or cycle is accepted.
func NewTree(rootRunID string, members []TreeMember) (Tree, error) {
	if strings.TrimSpace(rootRunID) == "" {
		return Tree{}, fmt.Errorf("%w: root run id is required", ErrInvalidTree)
	}
	if len(members) == 0 {
		return Tree{}, fmt.Errorf("%w: tree has no members", ErrInvalidTree)
	}

	builder := newRunTreeBuilder(rootRunID, len(members))
	for index, member := range members {
		if err := builder.addMember(index, member); err != nil {
			return Tree{}, err
		}
	}
	if _, rootFound := builder.lineages[rootRunID]; !rootFound {
		return Tree{}, fmt.Errorf("%w: root run %q is missing", ErrInvalidTree, rootRunID)
	}
	if err := builder.validateParents(); err != nil {
		return Tree{}, err
	}
	return builder.build()
}

func newRunTreeBuilder(rootRunID string, size int) *runTreeBuilder {
	return &runTreeBuilder{
		rootRunID: rootRunID,
		lineages:  make(map[string]Lineage, size),
		children:  make(map[string][]string, size),
		states:    make(map[string]uint8, size),
		tree: Tree{
			rootRunID: rootRunID,
			postorder: make([]string, 0, size),
			intervals: make(map[string]runTreeInterval, size),
		},
	}
}

func (r *runTreeBuilder) addMember(index int, member TreeMember) error {
	if err := member.Lineage.Validate(member.RunID); err != nil {
		return fmt.Errorf(
			"%w: member[%d] run %q lineage: %w",
			ErrInvalidTree,
			index,
			member.RunID,
			err,
		)
	}
	if _, duplicate := r.lineages[member.RunID]; duplicate {
		return fmt.Errorf("%w: duplicate run %q", ErrInvalidTree, member.RunID)
	}
	r.lineages[member.RunID] = member.Lineage

	if member.Lineage.IsRoot() {
		if member.RunID != r.rootRunID {
			return fmt.Errorf(
				"%w: run %q has root lineage, want only %q",
				ErrInvalidTree,
				member.RunID,
				r.rootRunID,
			)
		}
		return nil
	}
	if member.RunID == r.rootRunID {
		return fmt.Errorf(
			"%w: root run %q carries child lineage",
			ErrInvalidTree,
			r.rootRunID,
		)
	}
	if member.Lineage.RootRunID != r.rootRunID {
		return fmt.Errorf(
			"%w: child run %q names root %q, want %q",
			ErrInvalidTree,
			member.RunID,
			member.Lineage.RootRunID,
			r.rootRunID,
		)
	}
	r.children[member.Lineage.ParentRunID] = append(
		r.children[member.Lineage.ParentRunID],
		member.RunID,
	)
	return nil
}

func (r *runTreeBuilder) validateParents() error {
	for runID, lineage := range r.lineages {
		if lineage.IsRoot() {
			continue
		}
		if _, exists := r.lineages[lineage.ParentRunID]; !exists {
			return fmt.Errorf(
				"%w: child run %q names unknown parent %q",
				ErrInvalidTree,
				runID,
				lineage.ParentRunID,
			)
		}
	}
	for parentRunID := range r.children {
		slices.Sort(r.children[parentRunID])
	}
	return nil
}

func (r *runTreeBuilder) build() (Tree, error) {
	if err := r.visit(r.rootRunID); err != nil {
		return Tree{}, err
	}
	if len(r.tree.postorder) != len(r.lineages) {
		// With exactly one root-shaped member and every parent present, any
		// disconnected component necessarily contains a cycle. Traverse it so
		// callers receive the precise invariant violation.
		runIDs := make([]string, 0, len(r.lineages))
		for runID := range r.lineages {
			runIDs = append(runIDs, runID)
		}
		slices.Sort(runIDs)
		for _, runID := range runIDs {
			if r.states[runID] == runTreeUnvisited {
				if err := r.visit(runID); err != nil {
					return Tree{}, err
				}
			}
		}
		return Tree{}, fmt.Errorf("%w: tree contains a Run disconnected from root %q", ErrInvalidTree, r.rootRunID)
	}
	return r.tree, nil
}

func (r *runTreeBuilder) visit(runID string) error {
	switch r.states[runID] {
	case runTreeVisiting:
		return fmt.Errorf("%w: tree contains a cycle at run %q", ErrInvalidTree, runID)
	case runTreeVisited:
		return nil
	}
	r.states[runID] = runTreeVisiting
	start := len(r.tree.postorder)
	for _, childRunID := range r.children[runID] {
		if err := r.visit(childRunID); err != nil {
			return err
		}
	}
	r.tree.postorder = append(r.tree.postorder, runID)
	r.tree.intervals[runID] = runTreeInterval{start: start, end: len(r.tree.postorder)}
	r.states[runID] = runTreeVisited
	return nil
}

// Postorder returns a defensive copy of the complete canonical Run order.
func (t Tree) Postorder() []string {
	return slices.Clone(t.postorder)
}

// SubtreePostorder returns a defensive copy of runID and all its descendants in
// canonical postorder. The boolean is false when runID is not a tree member.
func (t Tree) SubtreePostorder(runID string) ([]string, bool) {
	interval, exists := t.intervals[runID]
	if !exists {
		return nil, false
	}
	return slices.Clone(t.postorder[interval.start:interval.end]), true
}
