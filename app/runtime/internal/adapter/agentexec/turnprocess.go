package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TurnProcess is the handle [Engine.StartTurn] returns. It exposes
// the underlying [runtime.Process] lifecycle (status, failure,
// cancellation) plus a typed result extractor — turn.Dispatcher drives
// the turn off Done() and queries Status() to decide TurnEnd reason.
//
// The interface lives in this package (not in the turn dispatcher) so
// test stubs can substitute a fake without standing up a full engine.
type TurnProcess interface {
	// ID is the underlying agent process id — surfaces to clients as
	// the turn handle so cancellation / resume requests route through
	// the runtime by process id.
	ID() string

	// Status reports the current [core.ProcessStatus] —
	// Running while the action loop ticks, Completed / Failed /
	// Killed / Terminated when the run ends.
	Status() core.ProcessStatus

	// Done delivers the final error (or nil on success) once the
	// run loop exits. Buffered cap-1 so callers can receive after
	// the goroutine has already finished.
	Done() <-chan error

	// Output extracts the typed [TurnOutput] from the process
	// blackboard. Returns an error when the run produced no output
	// (status reflects the terminal cause).
	Output() (TurnOutput, error)

	// Cancel marks the process [core.StatusKilled] via the engine.
	// The ongoing tick observes the status flip at its next checkpoint
	// and the run loop exits, delivering its error on Done().
	Cancel(ctx context.Context) error

	// Resume answers a HITL interrupt the process is parked on
	// (StatusWaiting) — a gated tool call or an ask_user / exit_plan_mode
	// question. It delivers the structured [interrupts.Resolution]
	// to the parked suspension and continues the process, returning a fresh
	// Done channel for the resumed run. Only valid while Status is
	// [core.StatusWaiting].
	Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error)

	// Suspension returns the HITL request the process is parked
	// on while StatusWaiting (a gated tool call or an ask_user /
	// exit_plan_mode question), or nil when nothing is parked. Its
	// Prompt JSON is what the client renders to make the decision.
	Suspension() *agent.Suspension

	// Discard releases a TERMINATED process: it removes the process from the
	// engine registry and deletes its persisted snapshot. With a ProcessStore
	// wired the runtime auto-snapshots every tick — including terminal
	// completion — but that snapshot only matters while the process is PARKED
	// awaiting HITL resume; once the turn reaches a terminal state it is dead
	// weight, and left behind it accumulates one orphaned snapshot row per run.
	// Cleanup failures don't rewrite the already-finished turn outcome, but are
	// returned so the owning turn span can retain them. Call exactly once at
	// terminal teardown — NEVER on a parked process, whose snapshot must survive
	// for resume.
	Discard(ctx context.Context) error
}

// turnProcess is the canonical [TurnProcess] backed by a real
// [runtime.Process]. It is package-private, so retaining the concrete Agent
// runtime keeps lifecycle commands inside this execution adapter.
type turnProcess struct {
	process *runtime.Process
	done    <-chan error
	engine  *runtime.Engine
}

func (p *turnProcess) ID() string                 { return p.process.ID() }
func (p *turnProcess) Status() core.ProcessStatus { return p.process.Status() }
func (p *turnProcess) Done() <-chan error         { return p.done }
func (p *turnProcess) Cancel(ctx context.Context) error {
	return p.engine.KillContext(ctx, p.process.ID())
}

func (p *turnProcess) Resume(ctx context.Context, resolution interrupts.Resolution) (<-chan error, error) {
	suspension := p.process.Suspension()
	if suspension == nil {
		return nil, fmt.Errorf("engine: process %s has no suspension", p.process.ID())
	}
	if err := p.engine.Resume(p.process.ID(), suspension.ID, resolution); err != nil {
		return nil, err
	}
	return p.engine.ContinueAsync(ctx, p.process.ID()), nil
}

func (p *turnProcess) Suspension() *agent.Suspension { return p.process.Suspension() }

func (p *turnProcess) Discard(ctx context.Context) error {
	if p == nil || p.process == nil || p.engine == nil {
		return errors.New("agentexec: discard process: incomplete turn process")
	}
	return discardProcessTree(ctx, p.process.ID(), p.engine)
}

type processTreeEngine interface {
	Processes() []*runtime.Process
	ProcessStore() core.ProcessStore
	KillContext(ctx context.Context, id string) error
	Remove(id string) error
}

// discardProcessTree removes descendants before parents in stable identity
// order. It combines the live registry with durable snapshots because either
// side may contain the only surviving edge after a crash or cancellation race.
func discardProcessTree(ctx context.Context, rootID string, engine processTreeEngine) error {
	if rootID == "" {
		return errors.New("agentexec: discard process tree: root process ID is empty")
	}
	if engine == nil {
		return errors.New("agentexec: discard process tree: engine is nil")
	}
	store := engine.ProcessStore()

	// Build the descendant graph from both live registry entries and durable
	// snapshots. A canceled nested tree may still have terminal child entries;
	// a crash-window cleanup may have already removed one from memory while its
	// snapshot remains. Walking both sources prevents either form from becoming
	// an orphan when the application discards the terminal root.
	children := make(map[string]map[string]struct{})
	live := make(map[string]*runtime.Process)
	parents := make(map[string]string)
	var cleanupErrs []error
	snapshotGraphComplete := true
	addChild := func(source, parentID, childID string) {
		if parentID == "" || childID == "" {
			return
		}
		if parentID == childID {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: %s process %q is its own parent", rootID, source, childID))
			snapshotGraphComplete = false
			return
		}
		if existing, ok := parents[childID]; ok && existing != parentID {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: %s process %q has parent %q, already linked to %q", rootID, source, childID, parentID, existing))
			snapshotGraphComplete = false
			return
		}
		parents[childID] = parentID
		if children[parentID] == nil {
			children[parentID] = make(map[string]struct{})
		}
		children[parentID][childID] = struct{}{}
	}
	for _, process := range engine.Processes() {
		if process == nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: live registry contains a nil process", rootID))
			continue
		}
		live[process.ID()] = process
		addChild("live", process.ParentID(), process.ID())
	}
	if store != nil {
		lister, ok := store.(core.SnapshotLister)
		if !ok {
			cleanupErrs = append(cleanupErrs, errors.New("agentexec: discard process tree: process store cannot list descendant snapshots"))
			snapshotGraphComplete = false
		} else if ids, err := lister.List(ctx); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: list snapshots: %w", rootID, err))
			snapshotGraphComplete = false
		} else {
			slices.Sort(ids)
			for _, id := range ids {
				snapshot, err := store.Load(ctx, id)
				if err != nil {
					if !errors.Is(err, core.ErrSnapshotNotFound) {
						cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: load snapshot %q: %w", rootID, id, err))
						snapshotGraphComplete = false
					}
					continue
				}
				if snapshot.ID != id {
					cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: snapshot key %q contains process %q", rootID, id, snapshot.ID))
					snapshotGraphComplete = false
					continue
				}
				addChild("snapshot", snapshot.ParentID, snapshot.ID)
			}
		}
	}

	var order []string
	visitState := make(map[string]uint8)
	var walk func(string)
	walk = func(id string) {
		switch visitState[id] {
		case 1:
			cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: descendant cycle reaches %q", rootID, id))
			snapshotGraphComplete = false
			return
		case 2:
			return
		}
		visitState[id] = 1
		childIDs := make([]string, 0, len(children[id]))
		for childID := range children[id] {
			childIDs = append(childIDs, childID)
		}
		slices.Sort(childIDs)
		for _, childID := range childIDs {
			walk(childID)
		}
		visitState[id] = 2
		order = append(order, id)
	}
	walk(rootID)

	deleter, canDelete := store.(core.SnapshotDeleter)
	if store != nil && !canDelete {
		cleanupErrs = append(cleanupErrs, errors.New("agentexec: discard process tree: process store cannot delete terminal snapshots"))
	}
	blocked := make(map[string]bool)
	for _, id := range order {
		for childID := range children[id] {
			if blocked[childID] {
				blocked[id] = true
				break
			}
		}
		if blocked[id] {
			continue
		}
		if process := live[id]; process != nil && !process.Status().IsTerminal() {
			if err := engine.KillContext(ctx, id); err != nil && !errors.Is(err, runtime.ErrProcessNotFound) {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: kill process %q: %w", rootID, id, err))
				blocked[id] = true
				continue
			}
		}
		if live[id] != nil {
			if err := engine.Remove(id); err != nil && !errors.Is(err, runtime.ErrProcessNotFound) {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: remove process %q: %w", rootID, id, err))
				blocked[id] = true
				continue
			}
		}
		if canDelete && snapshotGraphComplete {
			if err := deleter.Delete(ctx, id); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("agentexec: discard process tree %q: delete snapshot %q: %w", rootID, id, err))
				blocked[id] = true
			}
		}
	}
	return errors.Join(cleanupErrs...)
}

func (p *turnProcess) Output() (TurnOutput, error) {
	output, ok := core.Result[TurnOutput](p.process)
	if ok {
		return output, nil
	}
	// Preserve the process failure's error chain when there is one (%w);
	// a bare %w on a nil failure would format as "%!w(<nil>)".
	if failure := p.process.Failure(); failure != nil {
		return TurnOutput{}, fmt.Errorf("engine: no TurnOutput produced (status=%s): %w", p.process.Status(), failure)
	}
	return TurnOutput{}, fmt.Errorf("engine: no TurnOutput produced (status=%s)", p.process.Status())
}
