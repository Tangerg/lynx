package sessions

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// parkedRunTerminalization derives the atomic Application write-set that ends
// one waiting Run tree. It combines a coherent Session snapshot with the exact
// Pending hand-off, but owns neither persistence nor executor cleanup.
type parkedRunTerminalization struct {
	sessionID  string
	rootRunID  string
	finishedAt time.Time
	outcome    rundomain.Outcome
	detail     string
	pending    runs.Pending
	snapshot   Snapshot
}

func (terminalization parkedRunTerminalization) build() (TerminalPlan, transcript.Run, error) {
	if err := terminalization.pending.Validate(); err != nil {
		return TerminalPlan{}, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: invalid Pending: %w",
			terminalization.rootRunID,
			err,
		)
	}
	if terminalization.outcome != rundomain.OutcomeCanceled &&
		terminalization.outcome != rundomain.OutcomeLost {
		return TerminalPlan{}, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: unsupported outcome %s",
			terminalization.rootRunID,
			terminalization.outcome,
		)
	}
	runsByID, rootAdmission, err := terminalization.indexRuns()
	if err != nil {
		return TerminalPlan{}, transcript.Run{}, err
	}
	terminalRuns, err := terminalization.terminalRuns(runsByID, rootAdmission)
	if err != nil {
		return TerminalPlan{}, transcript.Run{}, err
	}
	rootRun := terminalRuns[len(terminalRuns)-1]
	if rootRun.ID != terminalization.rootRunID || rootRun.GoalLeaseID != terminalization.pending.GoalLeaseID {
		return TerminalPlan{}, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root admission differs from Pending",
			terminalization.rootRunID,
		)
	}
	items, err := terminalization.terminalItems()
	if err != nil {
		return TerminalPlan{}, transcript.Run{}, err
	}
	root, ok := terminalization.pending.RootContinuation()
	if !ok {
		return TerminalPlan{}, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root continuation is missing",
			terminalization.rootRunID,
		)
	}
	plan := TerminalPlan{Runs: terminalRuns, Items: items, CheckpointRootID: root.MemberID}
	if rootRun.GoalLeaseID != "" {
		record := terminalGoalRun(rootRun)
		plan.GoalRun = &record
	}
	if err := plan.Validate(); err != nil {
		return TerminalPlan{}, transcript.Run{}, err
	}
	return plan, rootRun, nil
}

func (terminalization parkedRunTerminalization) indexRuns() (
	map[string]transcript.Run,
	transcript.Run,
	error,
) {
	runsByID := make(map[string]transcript.Run, len(terminalization.snapshot.Runs))
	for _, run := range terminalization.snapshot.Runs {
		if _, duplicate := runsByID[run.ID]; duplicate {
			return nil, transcript.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: duplicate Run %q",
				terminalization.rootRunID,
				run.ID,
			)
		}
		runsByID[run.ID] = run
	}
	rootAdmission, found := runsByID[terminalization.rootRunID]
	if !found || !rootAdmission.Lineage().IsRoot() {
		return nil, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root Run is missing",
			terminalization.rootRunID,
		)
	}
	if !terminalization.pending.Capabilities.Equal(rootAdmission.Capabilities) {
		return nil, transcript.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: Pending run capabilities differ from root Run admission",
			terminalization.rootRunID,
		)
	}
	pendingRunIDs := make(map[string]struct{}, len(terminalization.pending.Continuations))
	for _, continuation := range terminalization.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
	}
	for _, run := range terminalization.snapshot.Runs {
		if run.Lineage().TreeRootID(run.ID) != terminalization.rootRunID || run.State.IsTerminal() {
			continue
		}
		if _, covered := pendingRunIDs[run.ID]; !covered {
			return nil, transcript.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: non-terminal Run %q has no Pending continuation",
				terminalization.rootRunID,
				run.ID,
			)
		}
	}
	return runsByID, rootAdmission, nil
}

func (terminalization parkedRunTerminalization) terminalRuns(
	runsByID map[string]transcript.Run,
	rootAdmission transcript.Run,
) ([]transcript.Run, error) {
	terminalRuns := make([]transcript.Run, 0, len(terminalization.pending.Continuations))
	for _, continuation := range terminalization.pending.Continuations {
		run, found := runsByID[continuation.RunID]
		if !found {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q is missing",
				terminalization.rootRunID,
				continuation.RunID,
			)
		}
		if !waitingRunMatchesContinuation(run, continuation, terminalization.sessionID, rootAdmission.Capabilities) {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q differs from its continuation",
				terminalization.rootRunID,
				run.ID,
			)
		}
		state, ok := terminalization.terminalState(run.State)
		if !ok {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run %q: cannot apply outcome %s",
				run.ID,
				terminalization.outcome,
			)
		}
		run.State = state
		run.Outcome = &terminalization.outcome
		if terminalization.outcome == rundomain.OutcomeLost {
			run.Error = &transcript.Problem{
				Kind:   transcript.RunLostProblem,
				Scope:  transcript.RunProblem,
				Detail: "the parked Run tree's executor checkpoint could not be restored",
			}
		}
		run.Detail = terminalization.detail
		run.Interrupts = nil
		run.FinishedAt = terminalization.finishedAt.UTC()
		run.UpdatedAt = run.FinishedAt
		run.MessageMark = len(terminalization.snapshot.Messages)
		terminalRuns = append(terminalRuns, run)
	}
	return terminalRuns, nil
}

func waitingRunMatchesContinuation(
	run transcript.Run,
	continuation runs.Continuation,
	sessionID string,
	capabilities rundomain.RunCapabilities,
) bool {
	return run.SessionID == sessionID &&
		run.State == rundomain.Waiting &&
		run.Lineage() == continuation.Lineage &&
		run.ModelSelection == continuation.ModelSelection &&
		run.CreatedAt.Equal(continuation.RunCreatedAt) &&
		run.Metrics.Equal(continuation.Metrics) &&
		run.Limits == continuation.Limits &&
		run.Capabilities.Equal(capabilities)
}

func (terminalization parkedRunTerminalization) terminalState(state rundomain.RunState) (rundomain.RunState, bool) {
	if terminalization.outcome == rundomain.OutcomeLost {
		return state.RecoverLost()
	}
	return state.Terminate(terminalization.outcome)
}

func (terminalization parkedRunTerminalization) terminalItems() ([]transcript.Item, error) {
	interruptItems := make(map[string]string, len(terminalization.pending.Interrupts))
	for _, interruption := range terminalization.pending.Interrupts {
		interruptItems[interruption.ItemID] = interruption.RunID
	}
	pendingRunIDs := make(map[string]struct{}, len(terminalization.pending.Continuations))
	for _, continuation := range terminalization.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
	}
	items := make([]transcript.Item, 0, len(interruptItems))
	for _, item := range terminalization.snapshot.Items {
		ownerRunID, found := interruptItems[item.ID]
		if !found {
			if _, pending := pendingRunIDs[item.RunID]; pending && item.Status == transcript.ItemRunning {
				return nil, fmt.Errorf(
					"sessions: terminalize parked Run tree %q: Running Item %q has no matching interrupt",
					terminalization.rootRunID,
					item.ID,
				)
			}
			continue
		}
		if item.SessionID != terminalization.sessionID ||
			item.RunID != ownerRunID ||
			item.Status != transcript.ItemRunning {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: interrupt Item %q is not Running in Run %q",
				terminalization.rootRunID,
				item.ID,
				ownerRunID,
			)
		}
		item.Status = transcript.ItemIncomplete
		if item.Kind == transcript.ToolCall {
			item.FinishedAt = terminalization.finishedAt.UTC()
			if terminalization.outcome == rundomain.OutcomeLost {
				item.Error = &transcript.Problem{
					Kind:   transcript.ToolFailedProblem,
					Scope:  transcript.ToolProblem,
					Detail: "tool call abandoned because its run could not be resumed",
				}
			}
		}
		items = append(items, item)
		delete(interruptItems, item.ID)
	}
	if len(interruptItems) != 0 {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing an interrupt Item",
			terminalization.rootRunID,
		)
	}
	return items, nil
}

func terminalGoalRun(root transcript.Run) goal.RunRecord {
	record := goal.RunRecord{
		SessionID:   root.SessionID,
		LeaseID:     root.GoalLeaseID,
		RunID:       root.ID,
		Outcome:     *root.Outcome,
		Steps:       root.Metrics.Steps,
		CompletedAt: root.FinishedAt,
	}
	if root.Metrics.Usage != nil && root.Metrics.Usage.CostUSD != nil {
		record.CostUSD = *root.Metrics.Usage.CostUSD
	}
	return record
}
