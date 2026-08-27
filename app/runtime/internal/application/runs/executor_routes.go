package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

// executorRoute is the pump-local binding from one immutable executor member
// identity to its application Run. A child route is installed only after its
// opening transaction commits; every installed route owns one independent
// Segment reducer.
type executorRoute struct {
	member           ExecutorMember
	memberBound      bool
	runID            string
	segmentID        string
	rootRunID        string
	lineage          rundomain.Lineage
	modelSelection   modelref.Selection
	limits           rundomain.Limits
	capabilities     rundomain.Capabilities
	reducer          *reducer
	segmentStartedAt time.Time
	segmentFinished  bool
}

type executorRoutes struct {
	rootBound      bool
	root           *executorRoute
	byMember       map[string]*executorRoute
	byRunID        map[string]*executorRoute
	admissionOrder []*executorRoute
}

func (c *Coordinator) openingRoutes(
	spec segmentSpec,
	cancelReason func(string) string,
) (*executorRoutes, error) {
	if spec.ModelOnlyInput && (spec.GoalIncarnationID == "" || spec.Continuation != nil) {
		return nil, errors.New("runs: model-only opening input requires a fresh Goal root")
	}
	if spec.Continuation != nil {
		return c.resumedExecutorRoutes(spec, cancelReason)
	}
	if len(spec.Input) > 0 && (spec.ConversationInput == nil ||
		spec.ConversationInput.Role != corechat.RoleUser || spec.ConversationInput.Validate() != nil) {
		return nil, errors.New("runs: fresh root requires its exact composed conversation input")
	}
	rootReducer := newReducer(reducerConfig{
		RunID: spec.RunID, SegmentID: spec.SegmentID, SessionID: spec.SessionID,
		CWD: spec.CWD, ExecutorID: spec.ExecutorID, ModelSelection: spec.ModelSelection,
		GoalIncarnationID: spec.GoalIncarnationID,
		CreatedAt:         spec.CreatedAt, UserInput: spec.Input,
		ConversationInput: spec.ConversationInput, ModelOnlyInput: spec.ModelOnlyInput,
		Metrics: spec.priorMetrics(), Limits: spec.effectiveLimits(),
		Capabilities: spec.effectiveCapabilities(),
		Now:          c.publications.nowUTC, CancelReason: cancellationReason(cancelReason, spec.RunID),
	})
	root := &executorRoute{
		runID:          spec.RunID,
		segmentID:      spec.SegmentID,
		rootRunID:      spec.RunID,
		modelSelection: spec.ModelSelection,
		limits:         spec.effectiveLimits(),
		capabilities:   spec.effectiveCapabilities(),
		reducer:        rootReducer,
	}
	return &executorRoutes{
		root:           root,
		byMember:       make(map[string]*executorRoute),
		byRunID:        map[string]*executorRoute{root.runID: root},
		admissionOrder: []*executorRoute{root},
	}, nil
}

func (c *Coordinator) resumedExecutorRoutes(
	spec segmentSpec,
	cancelReason func(string) string,
) (*executorRoutes, error) {
	continuation := spec.Continuation
	if err := validateResumedRouteRequest(spec, continuation); err != nil {
		return nil, err
	}
	builder := resumedRouteBuilder{
		spec:         spec,
		continuation: continuation,
		cancelReason: cancelReason,
		newSegmentID: c.newSegmentID,
		now:          c.publications.nowUTC,
		routes: &executorRoutes{
			rootBound: true,
			byMember:  make(map[string]*executorRoute, len(continuation.continuations)),
			byRunID:   make(map[string]*executorRoute, len(continuation.continuations)),
		},
		segmentIDs: map[string]struct{}{spec.SegmentID: {}},
	}
	return builder.build()
}

func validateResumedRouteRequest(spec segmentSpec, continuation *treeContinuation) error {
	if continuation == nil {
		return errors.New("runs: resumed routes require a tree continuation")
	}
	if err := continuation.validate(); err != nil {
		return fmt.Errorf("runs: build resumed routes: %w", err)
	}
	if continuation.rootRunID != spec.RunID || continuation.sessionID != spec.SessionID {
		return fmt.Errorf(
			"runs: resumed route scope %q/%q does not match continuation %q/%q",
			spec.SessionID,
			spec.RunID,
			continuation.sessionID,
			continuation.rootRunID,
		)
	}
	if spec.SegmentID == "" {
		return errors.New("runs: resumed root segment id is required")
	}
	return nil
}

// resumedRouteBuilder reconstructs every per-Run reducer from one validated
// continuation and derives their deterministic admission order. Keeping the
// segment identity set with the construction state makes duplicate detection
// structural rather than a convention shared by unrelated helpers.
type resumedRouteBuilder struct {
	spec         segmentSpec
	continuation *treeContinuation
	cancelReason func(string) string
	newSegmentID func() string
	now          func() time.Time
	routes       *executorRoutes
	segmentIDs   map[string]struct{}
}

func (r *resumedRouteBuilder) build() (*executorRoutes, error) {
	for _, continuationState := range r.continuation.continuations {
		route, err := r.newRoute(continuationState)
		if err != nil {
			return nil, err
		}
		r.routes.byMember[route.member.MemberID] = route
		r.routes.byRunID[route.runID] = route
		if route.runID == r.continuation.rootRunID {
			r.routes.root = route
		}
	}
	if r.routes.root == nil {
		return nil, errors.New("runs: resumed tree has no root route")
	}
	if err := r.orderRoutesPreorder(); err != nil {
		return nil, err
	}
	return r.routes, nil
}

func (r *resumedRouteBuilder) newRoute(continuationState Continuation) (*executorRoute, error) {
	segmentID, err := r.segmentIDFor(continuationState.RunID)
	if err != nil {
		return nil, err
	}
	member := ExecutorMember{MemberID: continuationState.MemberID}
	if err := member.Validate(); err != nil {
		return nil, fmt.Errorf("runs: resumed Run %q member: %w", continuationState.RunID, err)
	}
	route := &executorRoute{
		member:         member,
		memberBound:    continuationState.Lineage.IsRoot(),
		runID:          continuationState.RunID,
		segmentID:      segmentID,
		rootRunID:      r.continuation.rootRunID,
		lineage:        continuationState.Lineage,
		modelSelection: continuationState.ModelSelection,
		limits:         continuationState.Limits,
		capabilities:   r.continuation.capabilities,
	}
	userInput := []transcript.ContentBlock(nil)
	goalIncarnationID := ""
	if continuationState.RunID == r.continuation.rootRunID {
		userInput = r.spec.Input
		goalIncarnationID = r.spec.GoalIncarnationID
	}
	route.reducer = newReducer(reducerConfig{
		RunID: route.runID, SegmentID: route.segmentID, SessionID: r.spec.SessionID,
		Lineage: route.lineage, CWD: r.spec.CWD, ExecutorID: r.spec.ExecutorID,
		GoalIncarnationID: goalIncarnationID, ModelSelection: route.modelSelection,
		CreatedAt: continuationState.RunCreatedAt, UserInput: userInput,
		Metrics: continuationState.Metrics, ContextTokens: continuationState.ContextTokens,
		Limits:       continuationState.Limits,
		Capabilities: r.continuation.capabilities, Continuation: r.continuation,
		Now:          r.now,
		CancelReason: cancellationReason(r.cancelReason, route.runID),
	})
	return route, nil
}

func (r *resumedRouteBuilder) segmentIDFor(runID string) (string, error) {
	if runID == r.continuation.rootRunID {
		return r.spec.SegmentID, nil
	}
	if r.newSegmentID == nil {
		return "", errors.New("runs: resumed child routes require a segment identity generator")
	}
	segmentID := r.newSegmentID()
	if segmentID == "" {
		return "", fmt.Errorf("runs: resumed child Run %q generated an empty segment id", runID)
	}
	if _, duplicate := r.segmentIDs[segmentID]; duplicate {
		return "", fmt.Errorf("runs: resumed tree generated duplicate segment %q", segmentID)
	}
	r.segmentIDs[segmentID] = struct{}{}
	return segmentID, nil
}

func (r *resumedRouteBuilder) orderRoutesPreorder() error {
	childrenByParentRunID := make(map[string][]*executorRoute, len(r.routes.byRunID))
	for _, route := range r.routes.byRunID {
		if route != r.routes.root {
			childrenByParentRunID[route.lineage.ParentRunID] = append(
				childrenByParentRunID[route.lineage.ParentRunID],
				route,
			)
		}
	}
	for parentRunID := range childrenByParentRunID {
		slices.SortFunc(childrenByParentRunID[parentRunID], func(left, right *executorRoute) int {
			return strings.Compare(left.runID, right.runID)
		})
	}
	var appendPreorder func(*executorRoute)
	appendPreorder = func(route *executorRoute) {
		r.routes.admissionOrder = append(r.routes.admissionOrder, route)
		for _, child := range childrenByParentRunID[route.runID] {
			appendPreorder(child)
		}
	}
	appendPreorder(r.routes.root)
	if len(r.routes.admissionOrder) != len(r.continuation.continuations) {
		return errors.New("runs: resumed route tree is disconnected")
	}
	return nil
}

// unfinishedInPostorder returns the active tree in contract publication order:
// descendants before ancestors, siblings by Run ID, root last.
func (e *executorRoutes) unfinishedInPostorder() ([]*executorRoute, error) {
	if e == nil || e.root == nil {
		return nil, errors.New("runs: executor routes have no root")
	}
	byRunID := make(map[string]*executorRoute, len(e.admissionOrder))
	members := make([]rundomain.TreeMember, 0, len(e.admissionOrder))
	for _, route := range e.admissionOrder {
		if route == nil {
			return nil, errors.New("runs: executor routes contain a nil route")
		}
		byRunID[route.runID] = route
		members = append(members, rundomain.TreeMember{
			RunID:   route.runID,
			Lineage: route.lineage,
		})
	}
	tree, err := rundomain.NewTree(e.root.runID, members)
	if err != nil {
		return nil, fmt.Errorf("runs: executor routes: %w", err)
	}
	ordered := make([]*executorRoute, 0, e.unfinishedCount())
	for _, runID := range tree.Postorder() {
		route := byRunID[runID]
		if route == nil {
			return nil, fmt.Errorf("runs: executor tree ordered unknown run %q", runID)
		}
		if !route.segmentFinished {
			if route.lineage.IsChild() {
				parent := byRunID[route.lineage.ParentRunID]
				if parent == nil {
					return nil, fmt.Errorf(
						"runs: active executor route %q has no parent route %q",
						route.runID,
						route.lineage.ParentRunID,
					)
				}
				if parent.segmentFinished {
					return nil, fmt.Errorf(
						"runs: active executor route %q descends from finished route %q",
						route.runID,
						parent.runID,
					)
				}
			}
			ordered = append(ordered, route)
		}
	}
	return ordered, nil
}

func (e *executorRoutes) unfinishedCount() int {
	count := 0
	for _, route := range e.admissionOrder {
		if !route.segmentFinished {
			count++
		}
	}
	return count
}

func (e *executorRoute) activeDuration(boundary time.Time) time.Duration {
	if e.segmentStartedAt.IsZero() || boundary.Before(e.segmentStartedAt) {
		return 0
	}
	return boundary.Sub(e.segmentStartedAt)
}

// resolve binds the first root member and then requires exact member stability.
// Child sources are never inferred from lineage alone: they become routable only
// after a conclusive child-start commit installs their exact identity.
func (e *executorRoutes) resolve(member ExecutorMember) (*executorRoute, error) {
	if member.Child() {
		if member.SpawnCallID == "" {
			return nil, fmt.Errorf("runs: child executor member %q has no spawn-call identity", member.MemberID)
		}
		route := e.byMember[member.MemberID]
		if route == nil {
			return nil, fmt.Errorf("runs: child executor member %q has no admitted child run", member.MemberID)
		}
		if !route.lineage.IsChild() {
			return nil, fmt.Errorf("runs: root Run %q emitted a child executor member", route.runID)
		}
		if !route.memberBound {
			parent := e.byRunID[route.lineage.ParentRunID]
			if parent == nil || parent.member.MemberID == "" {
				return nil, fmt.Errorf(
					"runs: resumed child Run %q has no bound parent Run %q",
					route.runID,
					route.lineage.ParentRunID,
				)
			}
			if member.ParentID != parent.member.MemberID {
				return nil, fmt.Errorf(
					"runs: resumed child executor member %q names parent %q, want Run %q member %q",
					member.MemberID,
					member.ParentID,
					parent.runID,
					parent.member.MemberID,
				)
			}
			route.member = member
			route.memberBound = true
		} else if route.member != member {
			return nil, fmt.Errorf("runs: child executor member %q changed immutable lineage", member.MemberID)
		}
		if route.segmentFinished {
			return nil, fmt.Errorf("runs: child executor member %q published after its segment finished", member.MemberID)
		}
		return route, nil
	}

	if !e.rootBound {
		e.rootBound = true
		e.root.member = member
		e.root.memberBound = true
		if member.MemberID != "" {
			e.byMember[member.MemberID] = e.root
		}
		return e.root, nil
	}
	if e.root.member != member {
		return nil, fmt.Errorf(
			"runs: root executor member changed from %q to %q",
			e.root.member.MemberID,
			member.MemberID,
		)
	}
	return e.root, nil
}

func (e *executorRoutes) parent(member ExecutorMember) (*executorRoute, error) {
	if !member.Child() {
		return nil, errors.New("runs: child opening request requires a child executor member")
	}
	if member.SpawnCallID == "" {
		return nil, fmt.Errorf("runs: child executor member %q has no spawn-call identity", member.MemberID)
	}
	if e.byMember[member.MemberID] != nil {
		return nil, fmt.Errorf("runs: child executor member %q opened more than once", member.MemberID)
	}
	parent := e.byMember[member.ParentID]
	if parent == nil {
		return nil, fmt.Errorf(
			"runs: child executor member %q references unknown parent member %q",
			member.MemberID,
			member.ParentID,
		)
	}
	if parent.member.MemberID != member.ParentID {
		return nil, fmt.Errorf("runs: child executor member %q parent route is inconsistent", member.MemberID)
	}
	if parent.reducer == nil {
		return nil, fmt.Errorf(
			"runs: child executor member %q parent member %q has no active reducer",
			member.MemberID,
			member.ParentID,
		)
	}
	if parent.segmentFinished {
		return nil, fmt.Errorf(
			"runs: child executor member %q parent member %q already finished",
			member.MemberID,
			member.ParentID,
		)
	}
	return parent, nil
}

func (e *executorRoutes) installChild(member ExecutorMember, route *executorRoute) {
	route.member = member
	route.memberBound = true
	e.byMember[member.MemberID] = route
	e.byRunID[route.runID] = route
	e.admissionOrder = append(e.admissionOrder, route)
}

func (e *executorRoutes) abortUnfinished() {
	for _, route := range e.admissionOrder {
		if !route.segmentFinished && route.reducer != nil {
			route.reducer.abort()
		}
	}
}

// validateRouteReductionBatch proves that a reducer cannot write or publish
// another route's facts. The reducer is application-owned, but this boundary is
// where one root journal multiplexes many Run/Segment identities; checking the
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
	if err := validateRouteCommit(route, sessionID, batch.parkCommit); err != nil {
		return err
	}
	for _, reduced := range batch.events {
		if err := validateRouteCommit(route, sessionID, reduced.Commit); err != nil {
			return err
		}
		if err := validateRoutedEvent(route, sessionID, reduced.Event); err != nil {
			return err
		}
	}
	return nil
}

func validateRouteCommit(route *executorRoute, sessionID string, commit *EventCommit) error {
	if commit == nil {
		return nil
	}
	if commit.RunID != route.runID || commit.SessionID != sessionID || commit.SegmentID != route.segmentID {
		return fmt.Errorf(
			"%w: route %q Segment %q commit targets Run %q Segment %q in session %q",
			errReducerInvariant,
			route.runID,
			route.segmentID,
			commit.RunID,
			commit.SegmentID,
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
	if commit.GoalRun != nil &&
		(commit.GoalRun.RunID != route.runID || commit.GoalRun.SessionID != sessionID) {
		return fmt.Errorf(
			"%w: route %q carries a Goal Run for run %q in session %q",
			errReducerInvariant,
			route.runID,
			commit.GoalRun.RunID,
			commit.GoalRun.SessionID,
		)
	}
	return nil
}

func validateRoutedEvent(route *executorRoute, sessionID string, routed RunEvent) error {
	switch event := routed.(type) {
	case SegmentStarted:
		return validateRouteRun(route, sessionID, event.Run)
	case SegmentFinished:
		return validateRouteRun(route, sessionID, event.Run)
	case ItemStarted:
		return validateRouteItemStart(route, sessionID, event.Item)
	case ItemCompleted:
		return validateRouteItem(route, sessionID, event.Item)
	case PlanSnapshot:
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
	return nil
}

func validateRouteItemStart(route *executorRoute, sessionID string, item ItemStart) error {
	if item.RunID != route.runID || item.SessionID != sessionID {
		return fmt.Errorf(
			"%w: route %q carries Item start %q for run %q in session %q",
			errReducerInvariant,
			route.runID,
			item.ItemID,
			item.RunID,
			item.SessionID,
		)
	}
	return item.validate()
}

func validateRouteRun(route *executorRoute, sessionID string, value rundomain.Run) error {
	if value.ID() != route.runID ||
		value.SessionID() != sessionID ||
		value.Lineage() != route.lineage {
		return fmt.Errorf(
			"%w: route %q carries run %q in session %q with lineage %+v; want session %q and lineage %+v",
			errReducerInvariant,
			route.runID,
			value.ID(),
			value.SessionID(),
			value.Lineage(),
			sessionID,
			route.lineage,
		)
	}
	if value.State() == rundomain.Running && value.ActiveSegmentID() != route.segmentID {
		return fmt.Errorf(
			"%w: route %q running segment is %q, want %q",
			errReducerInvariant,
			route.runID,
			value.ActiveSegmentID(),
			route.segmentID,
		)
	}
	return nil
}

func validateRouteItem(route *executorRoute, sessionID string, item transcript.Item) error {
	if item.RunID() != route.runID || item.SessionID() != sessionID {
		return fmt.Errorf(
			"%w: route %q carries item %q for run %q in session %q",
			errReducerInvariant,
			route.runID,
			item.ID(),
			item.RunID(),
			item.SessionID(),
		)
	}
	return nil
}

type preparedChildOpening struct {
	member      ExecutorMember
	route       *executorRoute
	batch       reductionBatch
	opening     OpeningCommit
	reservation ChildRunStartReservation
}

func (p *preparedChildOpening) releaseBinding(owner *runTreeOwner) {
	if p == nil || p.route == nil || owner == nil {
		return
	}
	owner.unbindExecutorMember(p.route.runID, p.member.MemberID)
}

// prepareChildOpening freezes the complete application projection for one
// prospective child. It performs no durable write and does not make the route
// observable; its caller owns either commit or releaseBinding.
func (c *Coordinator) prepareChildOpening(
	spec segmentSpec,
	owner *runTreeOwner,
	routes *executorRoutes,
	member ExecutorMember,
	startedAt time.Time,
) (*preparedChildOpening, error) {
	if startedAt.IsZero() {
		return nil, errors.New("runs: child opening has no executor start time")
	}
	parent, err := routes.parent(member)
	if err != nil {
		return nil, err
	}
	spawningItem, err := parent.reducer.spawningItem(member.SpawnCallID)
	if err != nil {
		return nil, fmt.Errorf(
			"runs: open child member %q from parent run %q: %w",
			member.MemberID,
			parent.runID,
			err,
		)
	}
	if spawningItem.SessionID() != spec.SessionID {
		return nil, fmt.Errorf(
			"runs: open child member %q spawning item belongs to session %q, want %q",
			member.MemberID,
			spawningItem.SessionID(),
			spec.SessionID,
		)
	}
	if validateErr := spawningItem.Validate(); validateErr != nil {
		return nil, fmt.Errorf("runs: open child member %q spawning item: %w", member.MemberID, validateErr)
	}
	childRunID := c.newRunID()
	if childRunID == "" {
		return nil, errors.New("runs: child opening generated an empty run id")
	}
	childSegmentID := c.newSegmentID()
	if childSegmentID == "" {
		return nil, errors.New("runs: child opening generated an empty segment id")
	}
	lineage := rundomain.Lineage{
		SpawnedByItemID: spawningItem.ID(),
		ParentRunID:     parent.runID,
		RootRunID:       parent.rootRunID,
	}
	if validateErr := lineage.Validate(childRunID); validateErr != nil {
		return nil, fmt.Errorf("runs: open child member %q lineage: %w", member.MemberID, validateErr)
	}
	startedAt = startedAt.UTC()
	child := &executorRoute{
		runID:          childRunID,
		segmentID:      childSegmentID,
		rootRunID:      parent.rootRunID,
		lineage:        lineage,
		modelSelection: parent.modelSelection,
		limits:         parent.limits,
		capabilities:   parent.capabilities,
	}
	child.reducer = newReducer(reducerConfig{
		RunID:          child.runID,
		SegmentID:      child.segmentID,
		SessionID:      spec.SessionID,
		Lineage:        child.lineage,
		CWD:            spec.CWD,
		ExecutorID:     spec.ExecutorID,
		ModelSelection: child.modelSelection,
		CreatedAt:      startedAt,
		Limits:         child.limits,
		Capabilities:   child.capabilities,
		Now:            c.publications.nowUTC,
		CancelReason:   cancellationReason(owner.CancelReasonFor, child.runID),
	})
	if bindExecutorMemberErr := owner.bindExecutorMember(child.runID, member.MemberID); bindExecutorMemberErr != nil {
		return nil, bindExecutorMemberErr
	}
	release := true
	defer func() {
		if release {
			owner.unbindExecutorMember(child.runID, member.MemberID)
		}
	}()
	projected, err := child.reducer.open()
	if err != nil {
		return nil, fmt.Errorf("runs: reduce child member %q opening: %w", member.MemberID, err)
	}
	if len(projected.events) == 0 || projected.parkCommit != nil {
		return nil, fmt.Errorf("runs: child member %q produced an invalid opening batch", member.MemberID)
	}
	opening := OpeningCommit{
		CommitID: newRunCommitID(),
		Admit: &rundomain.Draft{
			RunID:           child.runID,
			SessionID:       spec.SessionID,
			SpawnedByItemID: child.lineage.SpawnedByItemID,
			ParentRunID:     child.lineage.ParentRunID,
			RootRunID:       child.lineage.RootRunID,
			SegmentID:       child.segmentID,
			ModelSelection:  child.modelSelection,
			Limits:          child.limits,
			Capabilities:    child.capabilities,
			CreatedAt:       startedAt,
		},
		Events: []EventCommit{{
			RunID:     parent.runID,
			SessionID: spec.SessionID,
			SegmentID: parent.segmentID,
			Items:     []transcript.Item{spawningItem},
		}},
	}
	for _, reduced := range projected.events {
		if reduced.Event.Terminal() || reduced.Nudge != nil {
			return nil, fmt.Errorf("runs: child member %q produced an invalid opening event", member.MemberID)
		}
		if reduced.Commit != nil {
			opening.Events = append(opening.Events, *reduced.Commit)
		}
	}
	if err := validateRouteReductionBatch(child, spec.SessionID, projected); err != nil {
		return nil, err
	}
	reservation := ChildRunStartReservation{
		SessionID: spec.SessionID, ExecutorID: spec.ExecutorID, Member: member,
		Binding: ChildRunBinding{
			MemberID: member.MemberID, RunID: child.runID, ParentRunID: child.lineage.ParentRunID,
		},
		SegmentID: child.segmentID, SpawnedByItemID: spawningItem.ID(),
		RootRunID: child.rootRunID, StartedAt: startedAt,
	}
	if err := reservation.Validate(); err != nil {
		return nil, err
	}
	release = false
	return &preparedChildOpening{
		member: member, route: child, batch: projected, opening: opening, reservation: reservation,
	}, nil
}

func (c *Coordinator) activatePreparedChild(
	spec segmentSpec,
	routes *executorRoutes,
	prepared *preparedChildOpening,
) {
	prepared.route.segmentStartedAt = c.publications.nowUTC()
	routes.installChild(prepared.member, prepared.route)
	c.publications.publishRunMoved(spec.SessionID, prepared.route.runID)
}

func cancellationReason(resolve func(string) string, runID string) func() string {
	if resolve == nil {
		return nil
	}
	return func() string { return resolve(runID) }
}
