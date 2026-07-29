package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// executorRoute is the pump-local binding from one immutable executor process
// identity to its application Run. A child route is installed only after its
// opening transaction commits. Its reducer remains nil until B1.2c gives every
// child segment an independent state machine.
type executorRoute struct {
	source         ExecutorSource
	runID          string
	segmentID      string
	rootRunID      string
	modelSelection modelref.Selection
	limits         execution.RunLimits
	reducer        *reducer
}

type executorRoutes struct {
	rootBound bool
	root      *executorRoute
	byProcess map[string]*executorRoute
}

func newExecutorRoutes(spec segmentSpec, rootReducer *reducer) *executorRoutes {
	return &executorRoutes{
		root: &executorRoute{
			runID:          spec.RunID,
			segmentID:      spec.SegmentID,
			rootRunID:      spec.RunID,
			modelSelection: spec.ModelSelection,
			limits:         spec.effectiveLimits(),
			reducer:        rootReducer,
		},
		byProcess: make(map[string]*executorRoute),
	}
}

// resolve binds the first root source and then requires exact source stability.
// Child sources are never inferred from lineage alone: they become routable only
// after [Coordinator.openChildRun] commits and installs their exact identity.
func (routes *executorRoutes) resolve(source ExecutorSource) (*executorRoute, error) {
	if source.Child() {
		route := routes.byProcess[source.ProcessID]
		if route == nil {
			return nil, fmt.Errorf("runs: child executor source %q has no admitted child run", source.ProcessID)
		}
		if route.source != source {
			return nil, fmt.Errorf("runs: child executor source %q changed immutable lineage", source.ProcessID)
		}
		return route, nil
	}

	if !routes.rootBound {
		routes.rootBound = true
		routes.root.source = source
		if source.ProcessID != "" {
			routes.byProcess[source.ProcessID] = routes.root
		}
		return routes.root, nil
	}
	if routes.root.source != source {
		return nil, fmt.Errorf(
			"runs: root executor source changed from %q to %q",
			routes.root.source.ProcessID,
			source.ProcessID,
		)
	}
	return routes.root, nil
}

func (routes *executorRoutes) parent(source ExecutorSource) (*executorRoute, error) {
	if !source.Child() {
		return nil, errors.New("runs: child opening request requires a child executor source")
	}
	if source.SpawnCallID == "" {
		return nil, fmt.Errorf("runs: child executor source %q has no spawn-call identity", source.ProcessID)
	}
	if routes.byProcess[source.ProcessID] != nil {
		return nil, fmt.Errorf("runs: child executor process %q opened more than once", source.ProcessID)
	}
	parent := routes.byProcess[source.ParentID]
	if parent == nil {
		return nil, fmt.Errorf(
			"runs: child executor source %q references unknown parent process %q",
			source.ProcessID,
			source.ParentID,
		)
	}
	if parent.source.ProcessID != source.ParentID {
		return nil, fmt.Errorf("runs: child executor source %q parent route is inconsistent", source.ProcessID)
	}
	if parent.reducer == nil {
		return nil, fmt.Errorf(
			"runs: child executor source %q parent process %q has no active reducer",
			source.ProcessID,
			source.ParentID,
		)
	}
	return parent, nil
}

func (routes *executorRoutes) installChild(source ExecutorSource, route *executorRoute) {
	route.source = source
	routes.byProcess[source.ProcessID] = route
}

// openChildRun atomically persists the parent's running spawning Item and the
// child Run that references it. Installing the in-memory route and publishing
// invalidation happen only after that write-set commits.
func (c *Coordinator) openChildRun(
	ctx context.Context,
	spec segmentSpec,
	routes *executorRoutes,
	source ExecutorSource,
	request ChildOpeningRequest,
) error {
	if err := request.validate(); err != nil {
		return err
	}
	parent, err := routes.parent(source)
	if err != nil {
		return err
	}
	spawningItem, err := parent.reducer.spawningItem(source.SpawnCallID)
	if err != nil {
		return fmt.Errorf(
			"runs: open child process %q from parent run %q: %w",
			source.ProcessID,
			parent.runID,
			err,
		)
	}
	spawningItem.SessionID = spec.SessionID
	if err := spawningItem.Validate(); err != nil {
		return fmt.Errorf("runs: open child process %q spawning item: %w", source.ProcessID, err)
	}
	if c.newRunID == nil || c.newSegmentID == nil {
		return errors.New("runs: child opening requires run and segment identity generators")
	}
	childRunID := c.newRunID()
	if childRunID == "" {
		return errors.New("runs: child opening generated an empty run id")
	}
	childSegmentID := c.newSegmentID()
	if childSegmentID == "" {
		return errors.New("runs: child opening generated an empty segment id")
	}
	child := &executorRoute{
		runID:          childRunID,
		segmentID:      childSegmentID,
		rootRunID:      parent.rootRunID,
		modelSelection: parent.modelSelection,
		limits:         parent.limits,
	}
	opening := OpeningCommit{
		Admit: &execution.RunDraft{
			RunID:           child.runID,
			SessionID:       spec.SessionID,
			SpawnedByItemID: spawningItem.ID,
			ParentRunID:     parent.runID,
			RootRunID:       child.rootRunID,
			SegmentID:       child.segmentID,
			ModelSelection:  child.modelSelection,
			Limits:          child.limits,
			CreatedAt:       request.StartedAt.UTC(),
		},
		Events: []EventCommit{{
			RunID:     parent.runID,
			SessionID: spec.SessionID,
			Items:     []transcript.Item{spawningItem},
		}},
	}
	if err := c.effects.CommitOpening(ctx, opening); err != nil {
		return fmt.Errorf("runs: commit child process %q opening: %w", source.ProcessID, err)
	}
	routes.installChild(source, child)
	c.publishRunMoved(spec.SessionID, child.runID)
	return nil
}
