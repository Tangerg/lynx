package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// executorRoute is the pump-local binding from one immutable executor process
// identity to its application Run. A child route is installed only after its
// opening transaction commits; every installed route owns one independent
// Segment reducer.
type executorRoute struct {
	source           ExecutorSource
	runID            string
	segmentID        string
	rootRunID        string
	lineage          execution.RunLineage
	modelSelection   modelref.Selection
	limits           execution.RunLimits
	protocolProfile  execution.RunProtocolProfile
	reducer          *reducer
	segmentStartedAt time.Time
	segmentFinished  bool
}

type executorRoutes struct {
	rootBound      bool
	root           *executorRoute
	byProcess      map[string]*executorRoute
	admissionOrder []*executorRoute
}

func (c *Coordinator) openingRoutes(
	spec segmentSpec,
	cancelReason func() string,
) (*executorRoutes, error) {
	if spec.Pending != nil {
		return c.resumedExecutorRoutes(spec, cancelReason)
	}
	rootReducer := newReducer(reducerConfig{
		RunID: spec.RunID, SegmentID: spec.SegmentID, SessionID: spec.SessionID,
		Cwd: spec.Cwd, TurnID: spec.TurnID, ModelSelection: spec.ModelSelection,
		GoalLeaseID: spec.GoalLeaseID,
		CreatedAt:   spec.CreatedAt, UserInput: spec.Input,
		Metrics: spec.priorMetrics(), Limits: spec.effectiveLimits(),
		ProtocolProfile: spec.effectiveProfile(),
		Now:             c.now, CancelReason: cancelReason,
	})
	root := &executorRoute{
		runID:           spec.RunID,
		segmentID:       spec.SegmentID,
		rootRunID:       spec.RunID,
		modelSelection:  spec.ModelSelection,
		limits:          spec.effectiveLimits(),
		protocolProfile: spec.effectiveProfile(),
		reducer:         rootReducer,
	}
	return &executorRoutes{
		root:           root,
		byProcess:      make(map[string]*executorRoute),
		admissionOrder: []*executorRoute{root},
	}, nil
}

func (c *Coordinator) resumedExecutorRoutes(
	spec segmentSpec,
	cancelReason func() string,
) (*executorRoutes, error) {
	pending := spec.Pending
	if pending == nil {
		return nil, errors.New("runs: resumed routes require a pending barrier")
	}
	if err := pending.Validate(); err != nil {
		return nil, fmt.Errorf("runs: build resumed routes: %w", err)
	}
	if pending.RootRunID != spec.RunID || pending.SessionID != spec.SessionID {
		return nil, fmt.Errorf(
			"runs: resumed route scope %q/%q does not match pending %q/%q",
			spec.SessionID,
			spec.RunID,
			pending.SessionID,
			pending.RootRunID,
		)
	}
	if spec.SegmentID == "" {
		return nil, errors.New("runs: resumed root segment id is required")
	}
	routes := &executorRoutes{
		rootBound: true,
		byProcess: make(map[string]*executorRoute, len(pending.Continuations)),
	}
	byRunID := make(map[string]*executorRoute, len(pending.Continuations))
	segmentIDs := map[string]struct{}{spec.SegmentID: {}}
	for _, continuation := range pending.Continuations {
		segmentID := spec.SegmentID
		if continuation.RunID != pending.RootRunID {
			if c.newSegmentID == nil {
				return nil, errors.New("runs: resumed child routes require a segment identity generator")
			}
			segmentID = c.newSegmentID()
			if segmentID == "" {
				return nil, fmt.Errorf(
					"runs: resumed child Run %q generated an empty segment id",
					continuation.RunID,
				)
			}
			if _, duplicate := segmentIDs[segmentID]; duplicate {
				return nil, fmt.Errorf("runs: resumed tree generated duplicate segment %q", segmentID)
			}
			segmentIDs[segmentID] = struct{}{}
		}
		source := ExecutorSource{
			ProcessID:   continuation.ProcessID,
			ParentID:    continuation.ParentProcessID,
			SpawnCallID: continuation.SpawnCallID,
		}
		if err := source.Validate(); err != nil {
			return nil, fmt.Errorf("runs: resumed Run %q source: %w", continuation.RunID, err)
		}
		route := &executorRoute{
			source:           source,
			runID:            continuation.RunID,
			segmentID:        segmentID,
			rootRunID:        pending.RootRunID,
			lineage:          continuation.Lineage,
			modelSelection:   continuation.ModelSelection,
			limits:           continuation.Limits,
			protocolProfile:  pending.ProtocolProfile,
			segmentStartedAt: time.Time{},
		}
		userInput := []transcript.ContentBlock(nil)
		goalLeaseID := ""
		if continuation.RunID == pending.RootRunID {
			userInput = spec.Input
			goalLeaseID = spec.GoalLeaseID
		}
		route.reducer = newReducer(reducerConfig{
			RunID: route.runID, SegmentID: route.segmentID, SessionID: spec.SessionID,
			Lineage: route.lineage, Cwd: spec.Cwd, TurnID: spec.TurnID,
			GoalLeaseID: goalLeaseID, ModelSelection: route.modelSelection,
			CreatedAt: continuation.RunCreatedAt, UserInput: userInput,
			Metrics: continuation.Metrics, Limits: continuation.Limits,
			ProtocolProfile: pending.ProtocolProfile, Pending: pending,
			Now: c.now, CancelReason: cancelReason,
		})
		routes.byProcess[source.ProcessID] = route
		byRunID[route.runID] = route
		if route.runID == pending.RootRunID {
			routes.root = route
		}
	}
	if routes.root == nil {
		return nil, errors.New("runs: resumed tree has no root route")
	}
	children := make(map[string][]*executorRoute, len(byRunID))
	for _, route := range byRunID {
		if route == routes.root {
			continue
		}
		children[route.lineage.ParentRunID] = append(children[route.lineage.ParentRunID], route)
	}
	for parentRunID := range children {
		slices.SortFunc(children[parentRunID], func(left, right *executorRoute) int {
			return strings.Compare(left.runID, right.runID)
		})
	}
	var appendPreorder func(*executorRoute)
	appendPreorder = func(route *executorRoute) {
		routes.admissionOrder = append(routes.admissionOrder, route)
		for _, child := range children[route.runID] {
			appendPreorder(child)
		}
	}
	appendPreorder(routes.root)
	if len(routes.admissionOrder) != len(pending.Continuations) {
		return nil, errors.New("runs: resumed route tree is disconnected")
	}
	return routes, nil
}

// unfinishedInPostorder returns the active tree in contract publication order:
// descendants before ancestors, siblings by Run ID, root last.
func (routes *executorRoutes) unfinishedInPostorder() ([]*executorRoute, error) {
	children := make(map[string][]*executorRoute, len(routes.admissionOrder))
	for _, route := range routes.admissionOrder {
		if route == routes.root || route.segmentFinished {
			continue
		}
		children[route.lineage.ParentRunID] = append(children[route.lineage.ParentRunID], route)
	}
	for parentID := range children {
		slices.SortFunc(children[parentID], func(left, right *executorRoute) int {
			if left.runID < right.runID {
				return -1
			}
			if left.runID > right.runID {
				return 1
			}
			return 0
		})
	}
	var ordered []*executorRoute
	var visit func(*executorRoute) error
	visiting := make(map[string]bool, len(routes.admissionOrder))
	visited := make(map[string]bool, len(routes.admissionOrder))
	visit = func(route *executorRoute) error {
		if visiting[route.runID] {
			return fmt.Errorf("runs: executor routes contain a cycle at run %q", route.runID)
		}
		if visited[route.runID] {
			return nil
		}
		visiting[route.runID] = true
		for _, child := range children[route.runID] {
			if err := visit(child); err != nil {
				return err
			}
		}
		visiting[route.runID] = false
		visited[route.runID] = true
		if !route.segmentFinished {
			ordered = append(ordered, route)
		}
		return nil
	}
	if err := visit(routes.root); err != nil {
		return nil, err
	}
	if len(ordered) != routes.unfinishedCount() {
		return nil, errors.New("runs: executor routes contain an active Run disconnected from the root")
	}
	return ordered, nil
}

func (routes *executorRoutes) unfinishedCount() int {
	count := 0
	for _, route := range routes.admissionOrder {
		if !route.segmentFinished {
			count++
		}
	}
	return count
}

func (route *executorRoute) activeDuration(boundary time.Time) time.Duration {
	if route.segmentStartedAt.IsZero() || boundary.Before(route.segmentStartedAt) {
		return 0
	}
	return boundary.Sub(route.segmentStartedAt)
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
		if route.segmentFinished {
			return nil, fmt.Errorf("runs: child executor source %q published after its segment finished", source.ProcessID)
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
	if parent.segmentFinished {
		return nil, fmt.Errorf(
			"runs: child executor source %q parent process %q already finished",
			source.ProcessID,
			source.ParentID,
		)
	}
	return parent, nil
}

func (routes *executorRoutes) installChild(source ExecutorSource, route *executorRoute) {
	route.source = source
	routes.byProcess[source.ProcessID] = route
	routes.admissionOrder = append(routes.admissionOrder, route)
}

// unfinishedChildrenInReverseAdmission returns a reverse topological order:
// a descendant can only be admitted after its parent, so reversing admission
// always closes descendants before ancestors without reconstructing the tree.
func (routes *executorRoutes) unfinishedChildrenInReverseAdmission() []*executorRoute {
	children := make([]*executorRoute, 0, len(routes.admissionOrder)-1)
	for index := len(routes.admissionOrder) - 1; index >= 1; index-- {
		route := routes.admissionOrder[index]
		if !route.segmentFinished {
			children = append(children, route)
		}
	}
	return children
}

func (routes *executorRoutes) abortUnfinished() {
	for _, route := range routes.admissionOrder {
		if !route.segmentFinished && route.reducer != nil {
			route.reducer.abort()
		}
	}
}

// validateRouteReductionBatch proves that a reducer cannot write or publish
// another route's facts. The reducer is application-owned, but this boundary is
// where one root Journal multiplexes many Run/Segment identities; checking the
// complete batch before its first side effect keeps a future routing regression
// from becoming cross-Run transcript corruption.
func validateRouteReductionBatch(
	route *executorRoute,
	sessionID string,
	batch reductionBatch,
) error {
	if route.runID == "" || route.segmentID == "" {
		return fmt.Errorf("%w: executor route has incomplete run or segment identity", errReducerInvariant)
	}
	if err := route.lineage.Validate(route.runID); err != nil {
		return fmt.Errorf("%w: executor route %q lineage: %w", errReducerInvariant, route.runID, err)
	}
	validateCommit := func(commit *EventCommit) error {
		if commit == nil {
			return nil
		}
		if commit.RunID != route.runID || commit.SessionID != sessionID {
			return fmt.Errorf(
				"%w: route %q commit targets run %q in session %q",
				errReducerInvariant,
				route.runID,
				commit.RunID,
				commit.SessionID,
			)
		}
		for _, item := range commit.Items {
			if err := validateRouteItem(route, sessionID, item); err != nil {
				return err
			}
		}
		if commit.Run != nil {
			if err := validateRouteRun(route, sessionID, *commit.Run); err != nil {
				return err
			}
		}
		if commit.GoalTurn != nil &&
			(commit.GoalTurn.RunID != route.runID || commit.GoalTurn.SessionID != sessionID) {
			return fmt.Errorf(
				"%w: route %q carries a goal turn for run %q in session %q",
				errReducerInvariant,
				route.runID,
				commit.GoalTurn.RunID,
				commit.GoalTurn.SessionID,
			)
		}
		return nil
	}

	if err := validateCommit(batch.parkCommit); err != nil {
		return err
	}
	for _, reduced := range batch.events {
		if err := validateCommit(reduced.Commit); err != nil {
			return err
		}
		switch event := reduced.Event.(type) {
		case SegmentStarted:
			if err := validateRouteRun(route, sessionID, event.Run); err != nil {
				return err
			}
		case SegmentFinished:
			if err := validateRouteRun(route, sessionID, event.Run); err != nil {
				return err
			}
		case ItemStarted:
			if err := validateRouteItem(route, sessionID, event.Item); err != nil {
				return err
			}
		case ItemCompleted:
			if err := validateRouteItem(route, sessionID, event.Item); err != nil {
				return err
			}
		case StateSnapshot:
			if event.SessionID != sessionID {
				return fmt.Errorf(
					"%w: route %q carries state for session %q, want %q",
					errReducerInvariant,
					route.runID,
					event.SessionID,
					sessionID,
				)
			}
		}
	}
	return nil
}

func validateRouteRun(route *executorRoute, sessionID string, run transcript.Run) error {
	if run.ID != route.runID ||
		run.SessionID != sessionID ||
		run.Lineage() != route.lineage {
		return fmt.Errorf(
			"%w: route %q carries run %q in session %q with lineage %+v; want session %q and lineage %+v",
			errReducerInvariant,
			route.runID,
			run.ID,
			run.SessionID,
			run.Lineage(),
			sessionID,
			route.lineage,
		)
	}
	if run.State == execution.Running && run.ActiveSegmentID != route.segmentID {
		return fmt.Errorf(
			"%w: route %q running segment is %q, want %q",
			errReducerInvariant,
			route.runID,
			run.ActiveSegmentID,
			route.segmentID,
		)
	}
	return nil
}

func validateRouteItem(route *executorRoute, sessionID string, item transcript.Item) error {
	if item.RunID != route.runID || item.SessionID != sessionID {
		return fmt.Errorf(
			"%w: route %q carries item %q for run %q in session %q",
			errReducerInvariant,
			route.runID,
			item.ID,
			item.RunID,
			item.SessionID,
		)
	}
	return nil
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
	cancelReason func() string,
) (*executorRoute, reductionBatch, error) {
	if err := request.validate(); err != nil {
		return nil, reductionBatch{}, err
	}
	parent, err := routes.parent(source)
	if err != nil {
		return nil, reductionBatch{}, err
	}
	spawningItem, err := parent.reducer.spawningItem(source.SpawnCallID)
	if err != nil {
		return nil, reductionBatch{}, fmt.Errorf(
			"runs: open child process %q from parent run %q: %w",
			source.ProcessID,
			parent.runID,
			err,
		)
	}
	spawningItem.SessionID = spec.SessionID
	if err := spawningItem.Validate(); err != nil {
		return nil, reductionBatch{}, fmt.Errorf("runs: open child process %q spawning item: %w", source.ProcessID, err)
	}
	if c.newRunID == nil || c.newSegmentID == nil {
		return nil, reductionBatch{}, errors.New("runs: child opening requires run and segment identity generators")
	}
	childRunID := c.newRunID()
	if childRunID == "" {
		return nil, reductionBatch{}, errors.New("runs: child opening generated an empty run id")
	}
	childSegmentID := c.newSegmentID()
	if childSegmentID == "" {
		return nil, reductionBatch{}, errors.New("runs: child opening generated an empty segment id")
	}
	lineage := execution.RunLineage{
		SpawnedByItemID: spawningItem.ID,
		ParentRunID:     parent.runID,
		RootRunID:       parent.rootRunID,
	}
	if err := lineage.Validate(childRunID); err != nil {
		return nil, reductionBatch{}, fmt.Errorf("runs: open child process %q lineage: %w", source.ProcessID, err)
	}
	startedAt := request.StartedAt.UTC()
	child := &executorRoute{
		runID:           childRunID,
		segmentID:       childSegmentID,
		rootRunID:       parent.rootRunID,
		lineage:         lineage,
		modelSelection:  parent.modelSelection,
		limits:          parent.limits,
		protocolProfile: parent.protocolProfile,
	}
	child.reducer = newReducer(reducerConfig{
		RunID:           child.runID,
		SegmentID:       child.segmentID,
		SessionID:       spec.SessionID,
		Lineage:         child.lineage,
		Cwd:             spec.Cwd,
		TurnID:          spec.TurnID,
		ModelSelection:  child.modelSelection,
		CreatedAt:       startedAt,
		Limits:          child.limits,
		ProtocolProfile: child.protocolProfile,
		Now:             c.now,
		CancelReason:    cancelReason,
	})
	projected, err := child.reducer.open()
	if err != nil {
		return nil, reductionBatch{}, fmt.Errorf("runs: reduce child process %q opening: %w", source.ProcessID, err)
	}
	if len(projected.events) == 0 || projected.parkCommit != nil {
		return nil, reductionBatch{}, fmt.Errorf("runs: child process %q produced an invalid opening batch", source.ProcessID)
	}
	opening := OpeningCommit{
		Admit: &execution.RunDraft{
			RunID:           child.runID,
			SessionID:       spec.SessionID,
			SpawnedByItemID: child.lineage.SpawnedByItemID,
			ParentRunID:     child.lineage.ParentRunID,
			RootRunID:       child.lineage.RootRunID,
			SegmentID:       child.segmentID,
			ModelSelection:  child.modelSelection,
			Limits:          child.limits,
			CreatedAt:       startedAt,
		},
		Events: []EventCommit{{
			RunID:     parent.runID,
			SessionID: spec.SessionID,
			Items:     []transcript.Item{spawningItem},
		}},
	}
	for _, reduced := range projected.events {
		if reduced.Event.Terminal() || reduced.Nudge != nil {
			return nil, reductionBatch{}, fmt.Errorf("runs: child process %q produced an invalid opening event", source.ProcessID)
		}
		if reduced.Commit != nil {
			opening.Events = append(opening.Events, *reduced.Commit)
		}
	}
	if err := c.effects.CommitOpening(ctx, opening); err != nil {
		return nil, reductionBatch{}, fmt.Errorf("runs: commit child process %q opening: %w", source.ProcessID, err)
	}
	child.segmentStartedAt = c.now().UTC()
	routes.installChild(source, child)
	c.publishRunMoved(spec.SessionID, child.runID)
	return child, projected, nil
}
