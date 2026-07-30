package runs

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// cancellationRun is one immutable member of a command-bound cancellation
// plan. Source is present only for a non-terminal Run whose executor process
// must remain addressable at this boundary.
type cancellationRun struct {
	run       transcript.Run
	source    ExecutorSource
	hasSource bool
}

// cancellationPlan is the complete, immutable fact set one runs.cancel command
// acts on. It is application-private because it combines domain Run topology,
// durable pending state, and process-local executor bindings; none of those
// outer representations belongs in the execution domain itself.
type cancellationPlan struct {
	root                 cancellationRun
	target               cancellationRun
	targetSubtree        []cancellationRun
	survivingTree        []cancellationRun
	treeState            execution.RunState
	turn                 execution.TurnRef
	pending              interrupts.Pending
	hasPending           bool
	spawningItem         transcript.Item
	hasSpawningItem      bool
	targetInterruptItems []transcript.Item
	completePostorderIDs []string
}

// cancellationPlanFor resolves either a root or child address to one complete
// tree snapshot before any executor side effect. The single RunTree read avoids
// racing a target lookup against a second aggregate lookup.
func (c *Coordinator) cancellationPlanFor(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, liveSegment, bool, error) {
	runs, err := c.runs.RunTree(ctx, cmd.RunID)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	if len(runs) == 0 {
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf("%w: %q", ErrRunNotFound, cmd.RunID)
	}
	target, found := runByID(runs, cmd.RunID)
	if !found {
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
			"runs: tree containing target %q omitted the target",
			cmd.RunID,
		)
	}
	if target.Lineage().IsChild() && !cmd.AllowChildRun {
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf("%w: %q", ErrChildRunNotAllowed, cmd.RunID)
	}
	if target.State.IsTerminal() {
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf("%w: %q", ErrRunFinished, cmd.RunID)
	}

	rootRunID := target.Lineage().TreeRootID(target.ID)
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
			"runs: cancellation tree for Run %q omits root %q",
			cmd.RunID,
			rootRunID,
		)
	}
	pending, pendingFound, err := c.sessions.GetOpenInterrupt(ctx, rootRunID)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	live, liveFound := c.registry.Get(rootRunID)

	var (
		turn    execution.TurnRef
		sources map[string]ExecutorSource
	)
	switch root.State {
	case execution.Running:
		if pendingFound {
			return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
				"runs: running tree %q has an open interrupt",
				rootRunID,
			)
		}
		if !liveFound || live.handle == nil {
			// The terminal commit may have won after RunTree returned. Refresh
			// the addressed Run once so that race reports run_finished rather
			// than a false missing-owner invariant.
			refreshed, refreshedFound, refreshErr := c.runs.Run(ctx, cmd.RunID)
			switch {
			case refreshErr != nil:
				return cancellationPlan{}, liveSegment{}, false, refreshErr
			case !refreshedFound:
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: Run %q disappeared after its cancellation tree was resolved",
					cmd.RunID,
				)
			case refreshed.State.IsTerminal():
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"%w: %q completed as %s",
					ErrRunFinished,
					cmd.RunID,
					refreshed.State,
				)
			default:
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: running tree %q has no live root owner",
					rootRunID,
				)
			}
		}
		if err := validateCancellationLiveRoot(live, root); err != nil {
			return cancellationPlan{}, liveSegment{}, false, err
		}
		turn = execution.TurnRef{SessionID: live.record.SessionID, TurnID: live.record.TurnID}
		sources = live.handle.executorSourceSnapshot()
	case execution.Interrupted:
		if !pendingFound {
			return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
				"runs: interrupted tree %q has no open interrupt",
				rootRunID,
			)
		}
		if err := pending.Validate(); err != nil {
			return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
				"runs: cancellation tree %q pending set: %w",
				rootRunID,
				err,
			)
		}
		turn = execution.TurnRef{SessionID: pending.SessionID, TurnID: pending.TurnID}
		sources = make(map[string]ExecutorSource, len(pending.Continuations))
		for _, continuation := range pending.Continuations {
			sources[continuation.RunID] = ExecutorSource{
				ProcessID:   continuation.ProcessID,
				ParentID:    continuation.ParentProcessID,
				SpawnCallID: continuation.SpawnCallID,
			}
		}
		if liveFound {
			if live.handle == nil {
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: interrupted tree %q has a live registry entry without a handle",
					rootRunID,
				)
			}
			if err := validateCancellationLiveRoot(live, root); err != nil {
				return cancellationPlan{}, liveSegment{}, false, err
			}
			if live.record.TurnID != pending.TurnID {
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: interrupted tree %q live turn %q differs from pending turn %q",
					rootRunID,
					live.record.TurnID,
					pending.TurnID,
				)
			}
		}
	default:
		return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
			"runs: cancellation root %q has state %s while target %q remains non-terminal",
			rootRunID,
			root.State,
			cmd.RunID,
		)
	}

	var pendingPlan *interrupts.Pending
	if pendingFound {
		pendingCopy := pending
		pendingPlan = &pendingCopy
	}
	plan, err := newCancellationPlan(cmd.RunID, runs, turn, sources, pendingPlan)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	if plan.treeState == execution.Interrupted && plan.target.run.Lineage().IsChild() {
		if c.items == nil {
			return cancellationPlan{}, liveSegment{}, false, errors.New(
				"runs: transcript item projection is required for waiting child cancellation",
			)
		}
		item, found, err := c.items.Item(ctx, plan.target.run.SpawnedByItemID)
		if err != nil {
			return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
				"runs: read spawning Item %q: %w",
				plan.target.run.SpawnedByItemID,
				err,
			)
		}
		if !found {
			return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
				"runs: waiting child Run %q spawning Item %q is missing",
				plan.target.run.ID,
				plan.target.run.SpawnedByItemID,
			)
		}
		if err := validateWaitingCancellationSpawningItem(plan, item); err != nil {
			return cancellationPlan{}, liveSegment{}, false, err
		}
		plan.spawningItem = item
		plan.hasSpawningItem = true
		targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
		for _, member := range plan.targetSubtree {
			targetRunIDs[member.run.ID] = struct{}{}
		}
		for _, interrupt := range plan.pending.Interrupts {
			if _, targeted := targetRunIDs[interrupt.RunID]; !targeted {
				continue
			}
			item, found, err := c.items.Item(ctx, interrupt.ItemID)
			if err != nil {
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: read waiting interrupt Item %q for Run %q: %w",
					interrupt.ItemID,
					interrupt.RunID,
					err,
				)
			}
			if !found {
				return cancellationPlan{}, liveSegment{}, false, fmt.Errorf(
					"runs: waiting interrupt Item %q for Run %q is missing",
					interrupt.ItemID,
					interrupt.RunID,
				)
			}
			if err := validateWaitingCancellationInterruptItem(plan, interrupt, item); err != nil {
				return cancellationPlan{}, liveSegment{}, false, err
			}
			plan.targetInterruptItems = append(plan.targetInterruptItems, item)
		}
	}
	return plan, live, liveFound, nil
}

func validateWaitingCancellationInterruptItem(
	plan cancellationPlan,
	interrupt transcript.Interrupt,
	item transcript.Item,
) error {
	switch {
	case item.ID != interrupt.ItemID:
		return fmt.Errorf(
			"runs: waiting interrupt for Run %q resolved Item %q, want %q",
			interrupt.RunID,
			item.ID,
			interrupt.ItemID,
		)
	case item.SessionID != plan.root.run.SessionID:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Session %q, want %q",
			item.ID,
			item.SessionID,
			plan.root.run.SessionID,
		)
	case item.RunID != interrupt.RunID:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q belongs to Run %q, want %q",
			item.ID,
			item.RunID,
			interrupt.RunID,
		)
	case item.Status != transcript.ItemRunning:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q is in status %d, want running",
			item.ID,
			item.Status,
		)
	}
	switch interrupt.Kind {
	case execution.QuestionInterrupt:
		if item.Kind != transcript.QuestionItem ||
			item.Question == nil ||
			interrupt.Question == nil ||
			!reflect.DeepEqual(item.Question, interrupt.Question) {
			return fmt.Errorf(
				"runs: waiting question Item %q differs from its interrupt",
				item.ID,
			)
		}
	case execution.ApprovalInterrupt:
		if item.Kind != transcript.ToolCall ||
			item.Tool == nil ||
			interrupt.Approval == nil ||
			!reflect.DeepEqual(*item.Tool, interrupt.Approval.Tool) {
			return fmt.Errorf(
				"runs: waiting approval Item %q differs from its interrupt",
				item.ID,
			)
		}
	default:
		return fmt.Errorf(
			"runs: waiting interrupt Item %q has unsupported kind %s",
			item.ID,
			interrupt.Kind,
		)
	}
	return nil
}

func validateWaitingCancellationSpawningItem(plan cancellationPlan, item transcript.Item) error {
	switch {
	case item.ID != plan.target.run.SpawnedByItemID:
		return fmt.Errorf(
			"runs: waiting child Run %q resolved spawning Item %q, want %q",
			plan.target.run.ID,
			item.ID,
			plan.target.run.SpawnedByItemID,
		)
	case item.SessionID != plan.root.run.SessionID:
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Session %q, want %q",
			item.ID,
			item.SessionID,
			plan.root.run.SessionID,
		)
	case item.RunID != plan.target.run.ParentRunID:
		return fmt.Errorf(
			"runs: spawning Item %q belongs to Run %q, want parent Run %q",
			item.ID,
			item.RunID,
			plan.target.run.ParentRunID,
		)
	case item.Kind != transcript.ToolCall || item.Tool == nil:
		return fmt.Errorf("runs: spawning Item %q is not a tool call", item.ID)
	case item.Status != transcript.ItemIncomplete:
		return fmt.Errorf(
			"runs: spawning Item %q is in status %d, want incomplete",
			item.ID,
			item.Status,
		)
	case item.Error != nil:
		return fmt.Errorf("runs: spawning Item %q already carries a problem", item.ID)
	default:
		return nil
	}
}

func validateCancellationLiveRoot(live liveSegment, root transcript.Run) error {
	switch {
	case live.record.ID != root.ID:
		return fmt.Errorf(
			"runs: cancellation root %q is owned by registry entry %q",
			root.ID,
			live.record.ID,
		)
	case live.record.SessionID != root.SessionID:
		return fmt.Errorf(
			"runs: cancellation root %q belongs to session %q but its live owner belongs to %q",
			root.ID,
			root.SessionID,
			live.record.SessionID,
		)
	case live.record.TurnID == "":
		return fmt.Errorf("runs: cancellation root %q live owner has no turn id", root.ID)
	default:
		return nil
	}
}

func newCancellationPlan(
	targetRunID string,
	runs []transcript.Run,
	turn execution.TurnRef,
	sources map[string]ExecutorSource,
	pending *interrupts.Pending,
) (cancellationPlan, error) {
	target, found := runByID(runs, targetRunID)
	if !found {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is missing",
			targetRunID,
		)
	}
	rootRunID := target.Lineage().TreeRootID(target.ID)
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: root Run %q is missing",
			rootRunID,
		)
	}
	if err := turn.ValidateFor(root.SessionID); err != nil {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan for tree %q: turn: %w",
			rootRunID,
			err,
		)
	}

	byRunID := make(map[string]transcript.Run, len(runs))
	members := make([]execution.RunTreeMember, 0, len(runs))
	for _, run := range runs {
		if run.SessionID != root.SessionID {
			return cancellationPlan{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: Run %q belongs to session %q, want %q",
				rootRunID,
				run.ID,
				run.SessionID,
				root.SessionID,
			)
		}
		if _, duplicate := byRunID[run.ID]; duplicate {
			return cancellationPlan{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: duplicate Run %q",
				rootRunID,
				run.ID,
			)
		}
		byRunID[run.ID] = run
		members = append(members, execution.RunTreeMember{RunID: run.ID, Lineage: run.Lineage()})
	}
	tree, err := execution.NewRunTree(rootRunID, members)
	if err != nil {
		return cancellationPlan{}, fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	if root.State != execution.Running && root.State != execution.Interrupted {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: root Run %q is %s",
			rootRunID,
			root.State,
		)
	}
	if target.State.IsTerminal() {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is %s",
			targetRunID,
			target.State,
		)
	}
	for _, run := range runs {
		if !run.State.IsTerminal() && run.State != root.State {
			return cancellationPlan{}, fmt.Errorf(
				"runs: build cancellation plan: non-terminal Run %q is %s while root %q is %s",
				run.ID,
				run.State,
				rootRunID,
				root.State,
			)
		}
	}

	bindings, err := cancellationBindings(byRunID, sources)
	if err != nil {
		return cancellationPlan{}, err
	}
	openRunIDs := make([]string, 0, len(runs))
	for _, runID := range tree.Postorder() {
		run := byRunID[runID]
		if run.State.IsTerminal() {
			continue
		}
		openRunIDs = append(openRunIDs, runID)
		binding := bindings[runID]
		if run.Lineage().IsChild() && !binding.hasSource {
			return cancellationPlan{}, fmt.Errorf(
				"runs: build cancellation plan: non-terminal child Run %q has no executor binding",
				runID,
			)
		}
	}
	if err := validateCancellationProcessTree(byRunID, bindings); err != nil {
		return cancellationPlan{}, err
	}
	if err := validateCancellationPending(root, openRunIDs, bindings, pending); err != nil {
		return cancellationPlan{}, err
	}

	targetIDs, exists := tree.SubtreePostorder(targetRunID)
	if !exists {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is outside tree %q",
			targetRunID,
			rootRunID,
		)
	}
	targetSet := make(map[string]struct{}, len(targetIDs))
	for _, runID := range targetIDs {
		targetSet[runID] = struct{}{}
	}
	plan := cancellationPlan{
		root:                 bindings[rootRunID],
		target:               bindings[targetRunID],
		treeState:            root.State,
		turn:                 turn,
		hasPending:           pending != nil,
		completePostorderIDs: tree.Postorder(),
	}
	if pending != nil {
		plan.pending = *pending
	}
	for _, runID := range plan.completePostorderIDs {
		member := bindings[runID]
		if _, targeted := targetSet[runID]; targeted {
			plan.targetSubtree = append(plan.targetSubtree, member)
		} else {
			plan.survivingTree = append(plan.survivingTree, member)
		}
	}
	return plan, nil
}

func cancellationBindings(
	runs map[string]transcript.Run,
	sources map[string]ExecutorSource,
) (map[string]cancellationRun, error) {
	bindings := make(map[string]cancellationRun, len(runs))
	for runID, run := range runs {
		bindings[runID] = cancellationRun{run: run}
	}
	processOwners := make(map[string]string, len(sources))
	for runID, source := range sources {
		member, exists := bindings[runID]
		if !exists {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: executor binding names unknown Run %q",
				runID,
			)
		}
		if err := source.Validate(); err != nil {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: Run %q executor binding: %w",
				runID,
				err,
			)
		}
		if source.ProcessID == "" {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: Run %q executor binding has no process id",
				runID,
			)
		}
		if owner, duplicate := processOwners[source.ProcessID]; duplicate {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: process %q is bound to Runs %q and %q",
				source.ProcessID,
				owner,
				runID,
			)
		}
		processOwners[source.ProcessID] = runID
		member.source = source
		member.hasSource = true
		bindings[runID] = member
	}
	return bindings, nil
}

func validateCancellationProcessTree(
	runs map[string]transcript.Run,
	bindings map[string]cancellationRun,
) error {
	for runID, run := range runs {
		if run.State.IsTerminal() {
			continue
		}
		binding := bindings[runID]
		if run.Lineage().IsRoot() {
			if binding.hasSource && binding.source.Child() {
				return fmt.Errorf(
					"runs: build cancellation plan: root Run %q has child process %q",
					runID,
					binding.source.ProcessID,
				)
			}
			continue
		}
		if !binding.hasSource {
			continue
		}
		if !binding.source.Child() || binding.source.SpawnCallID == "" {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q has incomplete child process binding",
				runID,
			)
		}
		parent := bindings[run.ParentRunID]
		if !parent.hasSource {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q has no bound parent Run %q",
				runID,
				run.ParentRunID,
			)
		}
		if binding.source.ParentID != parent.source.ProcessID {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q process parent %q differs from parent Run %q process %q",
				runID,
				binding.source.ParentID,
				run.ParentRunID,
				parent.source.ProcessID,
			)
		}
	}
	return nil
}

func validateCancellationPending(
	root transcript.Run,
	openRunIDs []string,
	bindings map[string]cancellationRun,
	pending *interrupts.Pending,
) error {
	if root.State == execution.Running {
		if pending != nil {
			return fmt.Errorf(
				"runs: build cancellation plan: running tree %q carries a pending set",
				root.ID,
			)
		}
		return nil
	}
	if pending == nil {
		return fmt.Errorf(
			"runs: build cancellation plan: interrupted tree %q has no pending set",
			root.ID,
		)
	}
	if err := pending.Validate(); err != nil {
		return fmt.Errorf(
			"runs: build cancellation plan: pending tree %q: %w",
			root.ID,
			err,
		)
	}
	if pending.RootRunID != root.ID || pending.SessionID != root.SessionID {
		return fmt.Errorf(
			"runs: build cancellation plan: pending scope %q/%q differs from tree %q/%q",
			pending.SessionID,
			pending.RootRunID,
			root.SessionID,
			root.ID,
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
		if !binding.hasSource ||
			binding.source.ProcessID != continuation.ProcessID ||
			binding.source.ParentID != continuation.ParentProcessID ||
			binding.source.SpawnCallID != continuation.SpawnCallID {
			return fmt.Errorf(
				"runs: build cancellation plan: continuation Run %q differs from its executor binding",
				continuation.RunID,
			)
		}
	}
	return nil
}

func runByID(runs []transcript.Run, runID string) (transcript.Run, bool) {
	for _, run := range runs {
		if run.ID == runID {
			return run, true
		}
	}
	return transcript.Run{}, false
}
