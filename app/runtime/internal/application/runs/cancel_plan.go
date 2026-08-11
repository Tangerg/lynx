package runs

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// cancellationRun is one immutable member of a command-bound cancellation
// plan. MemberID is present only for a non-terminal Run whose executor member
// must remain addressable at this boundary; executor topology is not an
// application cancellation fact.
type cancellationRun struct {
	run       rundomain.Run
	memberID  string
	hasMember bool
}

// cancellationPlan is the complete, immutable fact set one cancellation command
// acts on. It is application-private because it combines domain Run topology,
// durable pending state, and process-local executor bindings; none of those
// outer representations belongs in the execution domain itself.
type cancellationPlan struct {
	root                 cancellationRun
	target               cancellationRun
	targetSubtree        []cancellationRun
	survivingTree        []cancellationRun
	treeState            rundomain.State
	executor             ExecutorRef
	pending              Pending
	hasPending           bool
	spawningItem         transcript.Item
	hasSpawningItem      bool
	targetInterruptItems []transcript.Item
	targetDrainedItems   []transcript.Item
	completePostorderIDs []string
}

// cancellationPlanSource is the coherent read model used to build one
// cancellation plan. It keeps repository facts and the process-local owner
// together only inside this use case; neither representation is promoted to a
// domain or executor contract.
type cancellationPlanSource struct {
	runs             []rundomain.Run
	pending          Pending
	hasPending       bool
	live             liveSegment
	hasLive          bool
	executor         ExecutorRef
	memberIDsByRunID map[string]string
}

// cancellationPlanFor resolves either a root or child address to one complete
// tree snapshot before any executor side effect. The single Tree read avoids
// racing a target lookup against a second aggregate lookup.
func (c *Coordinator) cancellationPlanFor(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, liveSegment, bool, error) {
	source, err := c.readCancellationPlanSource(ctx, cmd)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}

	var pending *Pending
	if source.hasPending {
		pending = &source.pending
	}
	plan, err := newCancellationPlan(
		cmd.RunID,
		source.runs,
		source.executor,
		source.memberIDsByRunID,
		pending,
	)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	if plan.treeState == rundomain.Waiting && plan.target.run.Lineage().IsChild() {
		if err := c.loadWaitingCancellationItems(ctx, &plan); err != nil {
			return cancellationPlan{}, liveSegment{}, false, err
		}
	}
	return plan, source.live, source.hasLive, nil
}

func (c *Coordinator) readCancellationPlanSource(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlanSource, error) {
	runs, err := c.runs.Tree(ctx, cmd.RunID)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	if len(runs) == 0 {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrRunNotFound, cmd.RunID)
	}
	target, found := runByID(runs, cmd.RunID)
	if !found {
		return cancellationPlanSource{}, fmt.Errorf(
			"runs: tree containing target %q omitted the target",
			cmd.RunID,
		)
	}
	if target.Lineage().IsChild() && !cmd.AllowChildRun {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrChildRunNotAllowed, cmd.RunID)
	}
	if target.State().IsTerminal() {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrRunFinished, cmd.RunID)
	}

	rootRunID := target.Lineage().TreeRootID(target.ID())
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationPlanSource{}, fmt.Errorf(
			"runs: cancellation tree for Run %q omits root %q",
			cmd.RunID,
			rootRunID,
		)
	}
	pending, pendingFound, err := c.interrupts.LookupOpenInterrupt(ctx, rootRunID)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	live, liveFound := c.registry.Get(rootRunID)
	executor, memberIDsByRunID, err := c.resolveCancellationOwner(
		ctx,
		cmd.RunID,
		root,
		pending,
		pendingFound,
		live,
		liveFound,
	)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	return cancellationPlanSource{
		runs:             runs,
		pending:          pending,
		hasPending:       pendingFound,
		live:             live,
		hasLive:          liveFound,
		executor:         executor,
		memberIDsByRunID: memberIDsByRunID,
	}, nil
}

func (c *Coordinator) resolveCancellationOwner(
	ctx context.Context,
	targetRunID string,
	root rundomain.Run,
	pending Pending,
	hasPending bool,
	live liveSegment,
	hasLive bool,
) (ExecutorRef, map[string]string, error) {
	switch root.State() {
	case rundomain.Running:
		if hasPending {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: running tree %q has an open interrupt",
					root.ID(),
				),
			)
		}
		if !hasLive || live.owner == nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: running tree %q has no live root owner",
					root.ID(),
				),
			)
		}
		if err := validateCancellationLiveRoot(live, root); err != nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx, targetRunID, root.State(), err,
			)
		}
		return ExecutorRef{
			SessionID:  live.record.SessionID,
			ExecutorID: live.record.ExecutorID,
		}, live.owner.executorMemberSnapshot(), nil
	case rundomain.Waiting:
		if !hasPending {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: waiting tree %q has no open interrupt",
					root.ID(),
				),
			)
		}
		if err := pending.Validate(); err != nil {
			return ExecutorRef{}, nil, fmt.Errorf(
				"runs: cancellation tree %q pending set: %w",
				root.ID(),
				err,
			)
		}
		members := make(map[string]string, len(pending.Continuations))
		for _, continuation := range pending.Continuations {
			members[continuation.RunID] = continuation.MemberID
		}
		if hasLive {
			if live.owner == nil {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx,
					targetRunID,
					root.State(),
					fmt.Errorf(
						"runs: waiting tree %q has a live registry entry without a Run-tree owner",
						root.ID(),
					),
				)
			}
			if err := validateCancellationLiveRoot(live, root); err != nil {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx, targetRunID, root.State(), err,
				)
			}
			if live.record.ExecutorID != pending.ExecutorID {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx,
					targetRunID,
					root.State(),
					fmt.Errorf(
						"runs: waiting tree %q live executor %q differs from pending executor %q",
						root.ID(),
						live.record.ExecutorID,
						pending.ExecutorID,
					),
				)
			}
		}
		return ExecutorRef{
			SessionID:  pending.SessionID,
			ExecutorID: pending.ExecutorID,
		}, members, nil
	default:
		return ExecutorRef{}, nil, fmt.Errorf(
			"runs: cancellation root %q has state %s while target %q remains non-terminal",
			root.ID(),
			root.State(),
			targetRunID,
		)
	}
}

// classifyCancellationOwnerDrift distinguishes a durable lifecycle transition
// from a persistent ownership invariant violation. Tree, interrupt, and live
// owner facts come from different consistency domains, so Resume or terminal
// commit can linearize between those reads. That loser is a normal busy/finished
// outcome; only a contradiction that still exists at the refreshed Run state is
// an internal invariant fault.
func (c *Coordinator) classifyCancellationOwnerDrift(
	ctx context.Context,
	targetRunID string,
	sourceState rundomain.State,
	cause error,
) error {
	refreshed, found, err := c.runs.Run(ctx, targetRunID)
	switch {
	case err != nil:
		return err
	case !found:
		return fmt.Errorf(
			"runs: Run %q disappeared after its cancellation tree was resolved: %w",
			targetRunID,
			cause,
		)
	case refreshed.State().IsTerminal():
		return fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			targetRunID,
			refreshed.State(),
		)
	case refreshed.State() != sourceState:
		return fmt.Errorf(
			"%w: Run %q moved from %s to %s while cancellation ownership was resolved: %v",
			ErrSessionBusy,
			targetRunID,
			sourceState,
			refreshed.State(),
			cause,
		)
	default:
		return cause
	}
}

func (c *Coordinator) loadWaitingCancellationItems(ctx context.Context, plan *cancellationPlan) error {
	if plan == nil {
		return errors.New("runs: waiting cancellation plan is required")
	}
	if c.items == nil {
		return errors.New("runs: transcript item projection is required for waiting child cancellation")
	}
	item, found, err := c.items.Item(ctx, plan.target.run.Lineage().SpawnedByItemID)
	if err != nil {
		return fmt.Errorf(
			"runs: read spawning Item %q: %w",
			plan.target.run.Lineage().SpawnedByItemID,
			err,
		)
	}
	if !found {
		return fmt.Errorf(
			"runs: waiting child Run %q spawning Item %q is missing",
			plan.target.run.ID(),
			plan.target.run.Lineage().SpawnedByItemID,
		)
	}
	if err := validateWaitingCancellationSpawningItem(*plan, item); err != nil {
		return err
	}
	plan.spawningItem = item
	plan.hasSpawningItem = true
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		targetRunIDs[member.run.ID()] = struct{}{}
	}
	seenToolItems := make(map[string]struct{})
	for _, request := range plan.pending.Interrupts {
		if _, targeted := targetRunIDs[request.RunID]; !targeted {
			continue
		}
		item, found, err := c.items.Item(ctx, request.ItemID)
		if err != nil {
			return fmt.Errorf(
				"runs: read waiting interrupt Item %q for Run %q: %w",
				request.ItemID,
				request.RunID,
				err,
			)
		}
		if !found {
			return fmt.Errorf(
				"runs: waiting interrupt Item %q for Run %q is missing",
				request.ItemID,
				request.RunID,
			)
		}
		if err := validateWaitingCancellationInterruptItem(*plan, request, item); err != nil {
			return err
		}
		plan.targetInterruptItems = append(plan.targetInterruptItems, item)
		if item.Kind() == transcript.ToolCall {
			seenToolItems[item.ID()] = struct{}{}
		}
	}
	for _, continuation := range plan.pending.Continuations {
		if _, targeted := targetRunIDs[continuation.RunID]; !targeted {
			continue
		}
		for _, drained := range continuation.DrainedTools {
			if _, duplicate := seenToolItems[drained.ItemID]; duplicate {
				return fmt.Errorf(
					"runs: waiting child cancellation Tool Item %q is both an interrupt and a drained tool",
					drained.ItemID,
				)
			}
			item, found, err := c.items.Item(ctx, drained.ItemID)
			if err != nil {
				return fmt.Errorf("runs: read waiting drained Tool Item %q: %w", drained.ItemID, err)
			}
			if !found {
				return fmt.Errorf("runs: waiting drained Tool Item %q is missing", drained.ItemID)
			}
			if err := validateWaitingCancellationDrainedItem(*plan, continuation, drained, item); err != nil {
				return err
			}
			seenToolItems[item.ID()] = struct{}{}
			plan.targetDrainedItems = append(plan.targetDrainedItems, item)
		}
	}
	return nil
}

func validateWaitingCancellationDrainedItem(
	plan cancellationPlan,
	continuation Continuation,
	drained DrainedTool,
	item transcript.Item,
) error {
	invocation, present := item.ToolInvocation()
	if item.ID() != drained.ItemID || item.SessionID() != plan.root.run.SessionID() ||
		item.RunID() != continuation.RunID || item.Kind() != transcript.ToolCall ||
		item.Status() != transcript.ItemRunning || !present ||
		invocation.Name != drained.Name || invocation.Arguments.Canonical() != drained.Arguments {
		return fmt.Errorf(
			"runs: waiting drained Tool Item %q differs from Run %q continuation",
			drained.ItemID,
			continuation.RunID,
		)
	}
	if _, failed := item.Failure(); failed {
		return fmt.Errorf("runs: waiting drained Tool Item %q already carries a failure", item.ID())
	}
	return nil
}

func validateWaitingCancellationInterruptItem(
	plan cancellationPlan,
	request transcript.Interrupt,
	item transcript.Item,
) error {
	switch {
	case item.ID() != request.ItemID:
		return fmt.Errorf(
			"runs: waiting interrupt for Run %q resolved Item %q, want %q",
			request.RunID,
			item.ID(),
			request.ItemID,
		)
	case item.SessionID() != plan.root.run.SessionID():
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Session %q, want %q",
			item.ID(),
			item.SessionID(),
			plan.root.run.SessionID(),
		)
	case item.RunID() != request.RunID:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Run %q, want %q",
			item.ID(),
			item.RunID(),
			request.RunID,
		)
	}
	switch request.Kind {
	case interrupt.Question:
		question, present := item.Question()
		if item.Kind() != transcript.QuestionItem || item.Status() != transcript.ItemCompleted ||
			!present ||
			request.Question == nil ||
			!reflect.DeepEqual(question, *request.Question) {
			return fmt.Errorf(
				"runs: waiting question Item %q differs from its interrupt",
				item.ID(),
			)
		}
	case interrupt.Approval:
		invocation, present := item.ToolInvocation()
		if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning ||
			!present ||
			request.Approval == nil ||
			!reflect.DeepEqual(invocation, request.Approval.Tool) {
			return fmt.Errorf(
				"runs: waiting approval Item %q differs from its interrupt",
				item.ID(),
			)
		}
	default:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q has unsupported kind %s",
			item.ID(),
			request.Kind,
		)
	}
	return nil
}

func validateWaitingCancellationSpawningItem(plan cancellationPlan, item transcript.Item) error {
	switch {
	case item.ID() != plan.target.run.Lineage().SpawnedByItemID:
		return fmt.Errorf(
			"runs: waiting child Run %q resolved spawning Item %q, want %q",
			plan.target.run.ID(),
			item.ID(),
			plan.target.run.Lineage().SpawnedByItemID,
		)
	case item.SessionID() != plan.root.run.SessionID():
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Session %q, want %q",
			item.ID(),
			item.SessionID(),
			plan.root.run.SessionID(),
		)
	case item.RunID() != plan.target.run.Lineage().ParentRunID:
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Run %q, want parent Run %q",
			item.ID(),
			item.RunID(),
			plan.target.run.Lineage().ParentRunID,
		)
	case item.Kind() != transcript.ToolCall:
		return fmt.Errorf("runs: spawning Item %q is not a tool call", item.ID())
	case item.Status() != transcript.ItemRunning:
		return fmt.Errorf(
			"runs: spawning Item %q is in status %s, want running",
			item.ID(),
			item.Status(),
		)
	}
	if _, present := item.ToolInvocation(); !present {
		return fmt.Errorf("runs: spawning Item %q has no tool invocation", item.ID())
	}
	if _, failed := item.Failure(); failed {
		return fmt.Errorf("runs: spawning Item %q already carries a failure", item.ID())
	}
	return nil
}

func validateCancellationLiveRoot(live liveSegment, root rundomain.Run) error {
	switch {
	case live.record.ID != root.ID():
		return fmt.Errorf(
			"runs: cancellation root %q is owned by registry entry %q",
			root.ID(),
			live.record.ID,
		)
	case live.record.SessionID != root.SessionID():
		return fmt.Errorf(
			"runs: cancellation root %q belongs to session %q but its live owner belongs to %q",
			root.ID(),
			root.SessionID(),
			live.record.SessionID,
		)
	case live.record.ExecutorID == "":
		return fmt.Errorf("runs: cancellation root %q live owner has no executor ID", root.ID())
	case live.record.SegmentID != root.ActiveSegmentID():
		return fmt.Errorf(
			"runs: cancellation root %q durable segment %q differs from live owner %q",
			root.ID(),
			root.ActiveSegmentID(),
			live.record.SegmentID,
		)
	case !live.record.CreatedAt.Equal(root.CreatedAt()):
		return fmt.Errorf("runs: cancellation root %q creation time differs from live owner", root.ID())
	case live.record.ModelSelection != root.ModelSelection():
		return fmt.Errorf("runs: cancellation root %q model selection differs from live owner", root.ID())
	case !live.record.Capabilities.Equal(root.Capabilities()):
		return fmt.Errorf("runs: cancellation root %q run capabilities differ from live owner", root.ID())
	default:
		return nil
	}
}

func newCancellationPlan(
	targetRunID string,
	runs []rundomain.Run,
	executor ExecutorRef,
	memberIDsByRunID map[string]string,
	pending *Pending,
) (cancellationPlan, error) {
	runTree, err := buildCancellationRunTree(targetRunID, runs, executor)
	if err != nil {
		return cancellationPlan{}, err
	}
	bindings, err := cancellationBindings(runTree.byRunID, memberIDsByRunID)
	if err != nil {
		return cancellationPlan{}, err
	}
	openRunIDs, err := runTree.openRunIDs(bindings)
	if err != nil {
		return cancellationPlan{}, err
	}
	if err := validateCancellationMemberBindings(runTree.byRunID, bindings); err != nil {
		return cancellationPlan{}, err
	}
	if err := validateCancellationPending(runTree.root, openRunIDs, bindings, pending); err != nil {
		return cancellationPlan{}, err
	}

	targetRunIDs, exists := runTree.topology.SubtreePostorder(targetRunID)
	if !exists {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is outside tree %q",
			targetRunID,
			runTree.root.ID(),
		)
	}
	targetRunIDSet := make(map[string]struct{}, len(targetRunIDs))
	for _, runID := range targetRunIDs {
		targetRunIDSet[runID] = struct{}{}
	}
	plan := cancellationPlan{
		root:                 bindings[runTree.root.ID()],
		target:               bindings[targetRunID],
		treeState:            runTree.root.State(),
		executor:             executor,
		hasPending:           pending != nil,
		completePostorderIDs: runTree.topology.Postorder(),
	}
	if pending != nil {
		plan.pending = *pending
	}
	for _, runID := range plan.completePostorderIDs {
		member := bindings[runID]
		if _, targeted := targetRunIDSet[runID]; targeted {
			plan.targetSubtree = append(plan.targetSubtree, member)
		} else {
			plan.survivingTree = append(plan.survivingTree, member)
		}
	}
	return plan, nil
}

// cancellationRunTree is the validated durable Run snapshot on which one
// cancellation decision is based. It owns product lifecycle and topology
// facts only; process-local executor bindings remain outside this value.
type cancellationRunTree struct {
	root     rundomain.Run
	target   rundomain.Run
	topology rundomain.Tree
	byRunID  map[string]rundomain.Run
}

func buildCancellationRunTree(
	targetRunID string,
	runs []rundomain.Run,
	executor ExecutorRef,
) (cancellationRunTree, error) {
	target, found := runByID(runs, targetRunID)
	if !found {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is missing",
			targetRunID,
		)
	}
	rootRunID := target.Lineage().TreeRootID(target.ID())
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan: root Run %q is missing",
			rootRunID,
		)
	}
	if err := executor.ValidateFor(root.SessionID()); err != nil {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan for tree %q: executor: %w",
			rootRunID,
			err,
		)
	}

	byRunID := make(map[string]rundomain.Run, len(runs))
	treeMembers := make([]rundomain.TreeMember, 0, len(runs))
	for index, run := range runs {
		if err := run.Validate(); err != nil {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: Run[%d] %q: %w",
				rootRunID,
				index,
				run.ID(),
				err,
			)
		}
		if run.SessionID() != root.SessionID() {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: Run %q belongs to session %q, want %q",
				rootRunID,
				run.ID(),
				run.SessionID(),
				root.SessionID(),
			)
		}
		if _, duplicate := byRunID[run.ID()]; duplicate {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: duplicate Run %q",
				rootRunID,
				run.ID(),
			)
		}
		byRunID[run.ID()] = run
		treeMembers = append(treeMembers, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
	}
	tree, err := rundomain.NewTree(rootRunID, treeMembers)
	if err != nil {
		return cancellationRunTree{}, fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	runTree := cancellationRunTree{
		root:     root,
		target:   target,
		topology: tree,
		byRunID:  byRunID,
	}
	if err := runTree.validateLifecycle(); err != nil {
		return cancellationRunTree{}, err
	}
	return runTree, nil
}

func (tree cancellationRunTree) validateLifecycle() error {
	root := tree.root
	if root.State() != rundomain.Running && root.State() != rundomain.Waiting {
		return fmt.Errorf(
			"runs: build cancellation plan: root Run %q is %s",
			root.ID(),
			root.State(),
		)
	}
	if tree.target.State().IsTerminal() {
		return fmt.Errorf(
			"runs: build cancellation plan: target Run %q is %s",
			tree.target.ID(),
			tree.target.State(),
		)
	}
	for _, run := range tree.byRunID {
		if !run.State().IsTerminal() && run.State() != root.State() {
			return fmt.Errorf(
				"runs: build cancellation plan: non-terminal Run %q is %s while root %q is %s",
				run.ID(),
				run.State(),
				root.ID(),
				root.State(),
			)
		}
	}
	return nil
}

func (tree cancellationRunTree) openRunIDs(
	bindings map[string]cancellationRun,
) ([]string, error) {
	openRunIDs := make([]string, 0, len(tree.byRunID))
	for _, runID := range tree.topology.Postorder() {
		run := tree.byRunID[runID]
		if run.State().IsTerminal() {
			continue
		}
		openRunIDs = append(openRunIDs, runID)
		binding := bindings[runID]
		if run.Lineage().IsChild() && !binding.hasMember {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: non-terminal child Run %q has no executor binding",
				runID,
			)
		}
	}
	return openRunIDs, nil
}

func cancellationBindings(
	runs map[string]rundomain.Run,
	memberIDsByRunID map[string]string,
) (map[string]cancellationRun, error) {
	bindings := make(map[string]cancellationRun, len(runs))
	for runID, run := range runs {
		bindings[runID] = cancellationRun{run: run}
	}
	memberOwners := make(map[string]string, len(memberIDsByRunID))
	for runID, memberID := range memberIDsByRunID {
		member, exists := bindings[runID]
		if !exists {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: executor binding names unknown Run %q",
				runID,
			)
		}
		if memberID == "" {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: Run %q executor binding has no member id",
				runID,
			)
		}
		if owner, duplicate := memberOwners[memberID]; duplicate {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: member %q is bound to Runs %q and %q",
				memberID,
				owner,
				runID,
			)
		}
		memberOwners[memberID] = runID
		member.memberID = memberID
		member.hasMember = true
		bindings[runID] = member
	}
	return bindings, nil
}

func validateCancellationMemberBindings(
	runs map[string]rundomain.Run,
	bindings map[string]cancellationRun,
) error {
	for runID, run := range runs {
		if run.State().IsTerminal() {
			continue
		}
		binding := bindings[runID]
		if run.Lineage().IsRoot() {
			continue
		}
		if !binding.hasMember {
			continue
		}
		parent := bindings[run.Lineage().ParentRunID]
		if !parent.hasMember {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q has no bound parent Run %q",
				runID,
				run.Lineage().ParentRunID,
			)
		}
	}
	return nil
}

func validateCancellationPending(
	root rundomain.Run,
	openRunIDs []string,
	bindings map[string]cancellationRun,
	pending *Pending,
) error {
	if root.State() == rundomain.Running {
		if pending != nil {
			return fmt.Errorf(
				"runs: build cancellation plan: running tree %q carries a pending set",
				root.ID(),
			)
		}
		return nil
	}
	if pending == nil {
		return fmt.Errorf(
			"runs: build cancellation plan: waiting tree %q has no pending set",
			root.ID(),
		)
	}
	if err := pending.Validate(); err != nil {
		return fmt.Errorf(
			"runs: build cancellation plan: pending tree %q: %w",
			root.ID(),
			err,
		)
	}
	activeRuns := make([]rundomain.Run, 0, len(openRunIDs))
	for _, runID := range openRunIDs {
		activeRuns = append(activeRuns, bindings[runID].run)
	}
	if err := validatePendingRunTree(*pending, activeRuns); err != nil {
		return fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	if pending.RootRunID != root.ID() || pending.SessionID != root.SessionID() {
		return fmt.Errorf(
			"runs: build cancellation plan: pending scope %q/%q differs from tree %q/%q",
			pending.SessionID,
			pending.RootRunID,
			root.SessionID(),
			root.ID(),
		)
	}
	if len(pending.Continuations) != len(openRunIDs) {
		return fmt.Errorf(
			"runs: build cancellation plan: %d pending continuations do not cover %d non-terminal Runs",
			len(pending.Continuations),
			len(openRunIDs),
		)
	}
	for index, continuation := range pending.Continuations {
		if continuation.RunID != openRunIDs[index] {
			return fmt.Errorf(
				"runs: build cancellation plan: continuation[%d] is Run %q, want %q",
				index,
				continuation.RunID,
				openRunIDs[index],
			)
		}
		binding := bindings[continuation.RunID]
		if !binding.hasMember || binding.memberID != continuation.MemberID {
			return fmt.Errorf(
				"runs: build cancellation plan: continuation Run %q differs from its executor binding",
				continuation.RunID,
			)
		}
	}
	return nil
}

func runByID(runs []rundomain.Run, runID string) (rundomain.Run, bool) {
	for _, run := range runs {
		if run.ID() == runID {
			return run, true
		}
	}
	return rundomain.Run{}, false
}
