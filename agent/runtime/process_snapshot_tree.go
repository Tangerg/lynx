package runtime

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
)

// SnapshotTree captures a stable, ownership-isolated snapshot of one complete
// registered process tree. processID must identify its root, and every process
// in the tree must be idle. The runtime performs no external I/O and returns an
// ordinary value.
func (e *Engine) SnapshotTree(ctx context.Context, processID string) (core.ProcessSnapshotTree, error) {
	if e == nil {
		return core.ProcessSnapshotTree{}, errors.New("runtime.Engine.SnapshotTree: nil Engine")
	}
	process, ok := e.processes.get(processID)
	if !ok {
		return core.ProcessSnapshotTree{}, processNotFoundError("snapshot process tree", processID)
	}
	if process.ParentID() != "" || process.depth != 0 {
		return core.ProcessSnapshotTree{}, fmt.Errorf(
			"runtime.Engine.SnapshotTree: process %q is not a process-tree root",
			processID,
		)
	}
	ctx = normalizeContext(ctx)
	releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(process))
	if err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("runtime.Engine.SnapshotTree: acquire process tree: %w", err)
	}
	if !e.processes.available(process) {
		releaseMutation()
		return core.ProcessSnapshotTree{}, processNotFoundError("snapshot process tree", processID)
	}
	claimed, err := e.claimSnapshotTree(process, make(map[string]struct{}))
	releaseMutation()
	if err != nil {
		return core.ProcessSnapshotTree{}, err
	}
	defer releaseSnapshotTree(claimed)

	var snapshots []core.ProcessSnapshot
	if err := captureSnapshotTree(claimed, &snapshots); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("runtime.Engine.SnapshotTree: %w", err)
	}
	tree := core.ProcessSnapshotTree{RootID: processID, Snapshots: snapshots}
	if err := tree.Validate(); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("runtime.Engine.SnapshotTree: %w", err)
	}
	if err := validateNestedSnapshotRelations(tree); err != nil {
		return core.ProcessSnapshotTree{}, fmt.Errorf("runtime.Engine.SnapshotTree: %w", err)
	}
	return tree, nil
}

type claimedSnapshotTree struct {
	process  *Process
	children []*claimedSnapshotTree
}

func (e *Engine) claimSnapshotTree(process *Process, visited map[string]struct{}) (*claimedSnapshotTree, error) {
	if process == nil {
		return nil, errors.New("runtime.Engine.SnapshotTree: process is nil")
	}
	if _, duplicate := visited[process.ID()]; duplicate {
		return nil, fmt.Errorf("%w: process tree cycle at %q", core.ErrInvalidSnapshot, process.ID())
	}
	visited[process.ID()] = struct{}{}
	if err := process.state.claimCheckpoint(false); err != nil {
		return nil, fmt.Errorf("runtime.Engine.SnapshotTree: claim process %q: %w", process.ID(), err)
	}
	tree := &claimedSnapshotTree{process: process}
	for _, child := range e.directChildren(process.ID()) {
		if child.depth != process.depth+1 {
			releaseSnapshotTree(tree)
			return nil, fmt.Errorf("%w: process %q depth does not follow parent %q", core.ErrInvalidSnapshot, child.ID(), process.ID())
		}
		childTree, err := e.claimSnapshotTree(child, visited)
		if err != nil {
			releaseSnapshotTree(tree)
			return nil, err
		}
		tree.children = append(tree.children, childTree)
	}
	return tree, nil
}

func captureSnapshotTree(tree *claimedSnapshotTree, snapshots *[]core.ProcessSnapshot) error {
	if tree == nil || tree.process == nil {
		return errors.New("claimed process tree is incomplete")
	}
	snapshot, err := tree.process.snapshotClaimed()
	if err != nil {
		return err
	}
	*snapshots = append(*snapshots, snapshot)
	for _, child := range tree.children {
		if err := captureSnapshotTree(child, snapshots); err != nil {
			return err
		}
	}
	return nil
}

func releaseSnapshotTree(tree *claimedSnapshotTree) {
	if tree == nil {
		return
	}
	for index := len(tree.children) - 1; index >= 0; index-- {
		releaseSnapshotTree(tree.children[index])
	}
	tree.process.state.releaseCheckpoint()
}

func validateNestedSnapshotRelations(tree core.ProcessSnapshotTree) error {
	byID := make(map[string]core.ProcessSnapshot, len(tree.Snapshots))
	for _, snapshot := range tree.Snapshots {
		byID[snapshot.ID] = snapshot
	}
	for _, parent := range tree.Snapshots {
		checkpoint, err := nestedChildrenFromSuspension(parent.Suspension)
		if err != nil {
			return fmt.Errorf("%w: process %q nested checkpoint: %w", core.ErrInvalidSnapshot, parent.ID, err)
		}
		for _, relation := range checkpoint.relations {
			child, ok := byID[relation.ChildID]
			if !ok {
				return fmt.Errorf("%w: nested child snapshot %q is missing", core.ErrInvalidSnapshot, relation.ChildID)
			}
			if err := relation.validateSnapshot(parent, child); err != nil {
				return fmt.Errorf("%w: process %q nested relation: %w", core.ErrInvalidSnapshot, parent.ID, err)
			}
		}
	}
	return nil
}

// RemoveTree releases a complete terminal process tree from the in-memory
// registry. It performs no external deletion; callers coordinate any external
// state before relinquishing runtime ownership.
func (e *Engine) RemoveTree(ctx context.Context, processID string) error {
	if e == nil {
		return errors.New("runtime.Engine.RemoveTree: nil Engine")
	}
	process, ok := e.Process(processID)
	if !ok {
		return processNotFoundError("remove process tree", processID)
	}
	if process.ParentID() != "" || process.depth != 0 {
		return fmt.Errorf("runtime.Engine.RemoveTree: process %q is not a process-tree root", processID)
	}
	ctx = normalizeContext(ctx)
	tree, err := e.discoverProcessTrees([]string{processID})
	if err != nil {
		return fmt.Errorf("runtime.Engine.RemoveTree: %w", err)
	}
	if err := tree.wait(ctx); err != nil {
		return fmt.Errorf("runtime.Engine.RemoveTree: %w", err)
	}
	releaseMutation, err := e.processMutations.acquire(ctx, e.processTreeRootID(process))
	if err != nil {
		return fmt.Errorf("runtime.Engine.RemoveTree: acquire process tree: %w", err)
	}
	defer releaseMutation()
	if err := tree.claim(); err != nil {
		return fmt.Errorf("runtime.Engine.RemoveTree: %w", err)
	}
	tree.release()
	return nil
}

func (e *Engine) processTreeRootID(process *Process) string {
	if process == nil {
		return ""
	}
	rootID := process.ID()
	visited := map[string]struct{}{rootID: {}}
	for parentID := process.ParentID(); parentID != ""; parentID = process.ParentID() {
		if _, duplicate := visited[parentID]; duplicate {
			break
		}
		parent, ok := e.Process(parentID)
		if !ok {
			break
		}
		visited[parentID] = struct{}{}
		rootID = parentID
		process = parent
	}
	return rootID
}

type discoveredProcessTrees struct {
	engine   *Engine
	order    []string
	live     map[string]*Process
	reserved bool
}

func (e *Engine) discoverProcessTrees(roots []string) (*discoveredProcessTrees, error) {
	discovered := &discoveredProcessTrees{engine: e, live: make(map[string]*Process)}
	if len(roots) == 0 {
		return discovered, nil
	}
	children := make(map[string]map[string]struct{})
	parents := make(map[string]string)
	addChild := func(parentID, childID string) error {
		if parentID == "" {
			return nil
		}
		if parentID == childID {
			return fmt.Errorf("runtime: discover process tree: process %q is its own parent", childID)
		}
		if previous, exists := parents[childID]; exists && previous != parentID {
			return fmt.Errorf("runtime: discover process tree: process %q has parent %q, already linked to %q", childID, parentID, previous)
		}
		parents[childID] = parentID
		if children[parentID] == nil {
			children[parentID] = make(map[string]struct{})
		}
		children[parentID][childID] = struct{}{}
		return nil
	}
	for _, process := range e.Processes() {
		if process == nil {
			return nil, errors.New("runtime: process registry contains nil process")
		}
		discovered.live[process.ID()] = process
		if err := addChild(process.ParentID(), process.ID()); err != nil {
			return nil, err
		}
	}
	visitState := make(map[string]uint8)
	var walk func(string) error
	walk = func(id string) error {
		switch visitState[id] {
		case 1:
			return fmt.Errorf("runtime: descendant cycle reaches %q", id)
		case 2:
			return nil
		}
		visitState[id] = 1
		for _, childID := range slices.Sorted(maps.Keys(children[id])) {
			if err := walk(childID); err != nil {
				return err
			}
		}
		visitState[id] = 2
		discovered.order = append(discovered.order, id)
		return nil
	}
	sortedRoots := slices.Clone(roots)
	slices.Sort(sortedRoots)
	for _, root := range sortedRoots {
		if strings.TrimSpace(root) == "" || strings.TrimSpace(root) != root {
			return nil, fmt.Errorf("runtime: invalid root process ID %q", root)
		}
		if err := walk(root); err != nil {
			return nil, err
		}
	}
	return discovered, nil
}

func (tree *discoveredProcessTrees) wait(ctx context.Context) error {
	if tree == nil {
		return nil
	}
	var errs []error
	for _, id := range tree.order {
		process := tree.live[id]
		if process == nil {
			continue
		}
		if !process.Status().IsTerminal() {
			errs = append(errs, fmt.Errorf("process %q: %w", id, ErrProcessActive))
			continue
		}
		if err := process.state.waitRun(ctx); err != nil {
			errs = append(errs, fmt.Errorf("wait for process %q: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (tree *discoveredProcessTrees) claim() error {
	if tree == nil {
		return nil
	}
	processes := tree.liveProcesses()
	if !tree.engine.processes.reserveProcesses(processes) {
		return fmt.Errorf("reserve process tree removal: %w", ErrProcessActive)
	}
	tree.reserved = true
	for _, process := range processes {
		if !process.state.removable() {
			tree.releaseClaims()
			return fmt.Errorf("claim process %q removal: %w", process.ID(), ErrProcessActive)
		}
	}
	return nil
}

func (tree *discoveredProcessTrees) releaseClaims() {
	if tree != nil && tree.reserved {
		tree.engine.processes.releaseProcesses(tree.liveProcesses())
		tree.reserved = false
	}
}

func (tree *discoveredProcessTrees) release() {
	if tree == nil || tree.engine == nil {
		return
	}
	processes := tree.liveProcesses()
	if !tree.engine.processes.unregisterReservedTree(processes) {
		tree.releaseClaims()
		panic("runtime: reserved process tree changed before release")
	}
	releaseProcessDeployments(processes)
	tree.reserved = false
}

func (tree *discoveredProcessTrees) liveProcesses() []*Process {
	processes := make([]*Process, 0, len(tree.order))
	for _, id := range tree.order {
		if process := tree.live[id]; process != nil {
			processes = append(processes, process)
		}
	}
	return processes
}
