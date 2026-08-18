package sessions

import (
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// parkedRunTerminalization derives the atomic Application write-set that ends
// one waiting Run tree. It combines a coherent Session snapshot with the exact
// Pending hand-off, but owns neither persistence nor executor cleanup.
type parkedRunTerminalization struct {
	sessionID     string
	rootRunID     string
	finishedAt    time.Time
	outcome       rundomain.Outcome
	detail        string
	pending       runs.Pending
	snapshot      Snapshot
	resumeClaimed bool
}

func (terminalization parkedRunTerminalization) build() (TerminalPlan, rundomain.Run, error) {
	if err := terminalization.pending.Validate(); err != nil {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: invalid Pending: %w",
			terminalization.rootRunID,
			err,
		)
	}
	if terminalization.outcome != rundomain.OutcomeCanceled &&
		terminalization.outcome != rundomain.OutcomeLost {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: unsupported outcome %s",
			terminalization.rootRunID,
			terminalization.outcome,
		)
	}
	runsByID, rootAdmission, err := terminalization.indexRuns()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	conversationMessages, err := terminalization.terminalConversationMessages()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	terminalRuns, err := terminalization.terminalRuns(
		runsByID,
		rootAdmission,
		len(terminalization.snapshot.Messages)+len(conversationMessages),
	)
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	rootRun := terminalRuns[len(terminalRuns)-1]
	if rootRun.ID() != terminalization.rootRunID || rootRun.GoalIncarnationID() != terminalization.pending.GoalIncarnationID {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root admission differs from Pending",
			terminalization.rootRunID,
		)
	}
	items, err := terminalization.terminalItems()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	root, ok := terminalization.pending.RootContinuation()
	if !ok {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root continuation is missing",
			terminalization.rootRunID,
		)
	}
	plan := TerminalPlan{
		Runs: terminalRuns, Items: items, Messages: conversationMessages,
		CheckpointRootID: root.MemberID, ResumeClaimed: terminalization.resumeClaimed,
	}
	if rootRun.GoalIncarnationID() != "" {
		record := terminalGoalRun(rootRun)
		plan.GoalRun = &record
	}
	if err := plan.Validate(); err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	return plan, rootRun, nil
}

func (terminalization parkedRunTerminalization) indexRuns() (
	map[string]rundomain.Run,
	rundomain.Run,
	error,
) {
	runsByID := make(map[string]rundomain.Run, len(terminalization.snapshot.Runs))
	for _, run := range terminalization.snapshot.Runs {
		if _, duplicate := runsByID[run.ID()]; duplicate {
			return nil, rundomain.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: duplicate Run %q",
				terminalization.rootRunID,
				run.ID(),
			)
		}
		runsByID[run.ID()] = run
	}
	rootAdmission, found := runsByID[terminalization.rootRunID]
	if !found || !rootAdmission.Lineage().IsRoot() {
		return nil, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root Run is missing",
			terminalization.rootRunID,
		)
	}
	if !terminalization.pending.Capabilities.Equal(rootAdmission.Capabilities()) {
		return nil, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: Pending run capabilities differ from root Run admission",
			terminalization.rootRunID,
		)
	}
	pendingRunIDs := make(map[string]struct{}, len(terminalization.pending.Continuations))
	for _, continuation := range terminalization.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
	}
	for _, run := range terminalization.snapshot.Runs {
		if run.Lineage().TreeRootID(run.ID()) != terminalization.rootRunID || run.State().IsTerminal() {
			continue
		}
		if _, covered := pendingRunIDs[run.ID()]; !covered {
			return nil, rundomain.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: non-terminal Run %q has no Pending continuation",
				terminalization.rootRunID,
				run.ID(),
			)
		}
	}
	return runsByID, rootAdmission, nil
}

func (terminalization parkedRunTerminalization) terminalRuns(
	runsByID map[string]rundomain.Run,
	rootAdmission rundomain.Run,
	messageMark int,
) ([]rundomain.Run, error) {
	terminalRuns := make([]rundomain.Run, 0, len(terminalization.pending.Continuations))
	for _, continuation := range terminalization.pending.Continuations {
		run, found := runsByID[continuation.RunID]
		if !found {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q is missing",
				terminalization.rootRunID,
				continuation.RunID,
			)
		}
		if !waitingRunMatchesContinuation(run, continuation, terminalization.sessionID, rootAdmission.Capabilities()) {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q differs from its continuation",
				terminalization.rootRunID,
				run.ID(),
			)
		}
		var terminal rundomain.Run
		var err error
		if terminalization.outcome == rundomain.OutcomeLost {
			terminal, err = run.RecoverLost(rundomain.Failure{
				Kind:   rundomain.FailureLost,
				Detail: "the parked Run tree's executor checkpoint could not be restored",
			}, terminalization.finishedAt, messageMark)
		} else {
			terminal, err = run.CancelWaiting(
				terminalization.detail,
				terminalization.finishedAt,
				messageMark,
			)
		}
		if err != nil {
			return nil, fmt.Errorf("sessions: terminalize parked Run %q: %w", run.ID(), err)
		}
		terminalRuns = append(terminalRuns, terminal)
	}
	return terminalRuns, nil
}

func waitingRunMatchesContinuation(
	run rundomain.Run,
	continuation runs.Continuation,
	sessionID string,
	capabilities rundomain.Capabilities,
) bool {
	return run.SessionID() == sessionID &&
		run.State() == rundomain.Waiting &&
		run.Lineage() == continuation.Lineage &&
		run.ModelSelection() == continuation.ModelSelection &&
		run.CreatedAt().Equal(continuation.RunCreatedAt) &&
		run.Metrics().Equal(continuation.Metrics) &&
		run.Limits() == continuation.Limits &&
		run.Capabilities().Equal(capabilities)
}

func (terminalization parkedRunTerminalization) terminalItems() ([]transcript.Item, error) {
	interruptItems := make(map[string]transcript.Interrupt, len(terminalization.pending.Interrupts))
	for _, interruption := range terminalization.pending.Interrupts {
		interruptItems[interruption.ItemID] = interruption
	}
	pendingRunIDs := make(map[string]struct{}, len(terminalization.pending.Continuations))
	drainedItems := make(map[string]runs.DrainedTool)
	for _, continuation := range terminalization.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
		for _, drained := range continuation.DrainedTools {
			drainedItems[drained.ItemID] = drained
		}
	}
	items := make([]transcript.Item, 0, len(interruptItems)+len(drainedItems))
	for _, item := range terminalization.snapshot.Items {
		request, found := interruptItems[item.ID()]
		if !found {
			if drained, drainedFound := drainedItems[item.ID()]; drainedFound {
				settled, changed, err := terminalization.terminalDrainedItem(item, drained)
				if err != nil {
					return nil, err
				}
				if changed {
					items = append(items, settled)
				}
				delete(drainedItems, item.ID())
				continue
			}
			if _, pending := pendingRunIDs[item.RunID()]; pending && item.Status() == transcript.ItemRunning {
				return nil, fmt.Errorf(
					"sessions: terminalize parked Run tree %q: Running Item %q has no matching interrupt",
					terminalization.rootRunID,
					item.ID(),
				)
			}
			continue
		}
		if item.SessionID() != terminalization.sessionID || item.RunID() != request.RunID {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: interrupt Item %q is not owned by Run %q",
				terminalization.rootRunID,
				item.ID(),
				request.RunID,
			)
		}
		switch request.Kind {
		case interrupt.Question:
			if item.Kind() != transcript.QuestionItem || item.Status() != transcript.ItemCompleted {
				return nil, fmt.Errorf("sessions: parked question Item %q is not a complete prompt", item.ID())
			}
		case interrupt.Approval:
			var failure *tool.Failure
			if terminalization.outcome == rundomain.OutcomeLost {
				failure = &tool.Failure{
					Kind:   tool.FailureExecution,
					Detail: "tool call abandoned because its run could not be resumed",
				}
			}
			settled, err := item.AbandonToolCall(failure, terminalization.finishedAt)
			if err != nil {
				return nil, fmt.Errorf("sessions: terminalize parked ToolCall %q: %w", item.ID(), err)
			}
			items = append(items, settled)
		default:
			return nil, fmt.Errorf("sessions: parked interrupt Item %q has unknown kind %s", item.ID(), request.Kind)
		}
		delete(interruptItems, item.ID())
	}
	if len(interruptItems) != 0 {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing an interrupt Item",
			terminalization.rootRunID,
		)
	}
	if len(drainedItems) != 0 {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing %d drained Tool Items",
			terminalization.rootRunID,
			len(drainedItems),
		)
	}
	return items, nil
}

func (terminalization parkedRunTerminalization) terminalDrainedItem(
	item transcript.Item,
	drained runs.DrainedTool,
) (transcript.Item, bool, error) {
	invocation, present := item.ToolInvocation()
	if item.SessionID() != terminalization.sessionID || item.Kind() != transcript.ToolCall ||
		!present || invocation.Name != drained.Name ||
		invocation.Arguments.Canonical() != drained.Arguments {
		return transcript.Item{}, false, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: drained Tool Item %q differs from its continuation",
			terminalization.rootRunID,
			item.ID(),
		)
	}
	switch item.Status() {
	case transcript.ItemRunning:
		var failure *tool.Failure
		if terminalization.outcome == rundomain.OutcomeLost {
			failure = &tool.Failure{
				Kind:   tool.FailureExecution,
				Detail: "tool call abandoned because its run could not be resumed",
			}
		}
		settled, err := item.AbandonToolCall(failure, terminalization.finishedAt)
		if err != nil {
			return transcript.Item{}, false, fmt.Errorf(
				"sessions: terminalize parked ToolCall %q: %w",
				item.ID(),
				err,
			)
		}
		return settled, true, nil
	default:
		return transcript.Item{}, false, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: drained Tool Item %q is %s",
			terminalization.rootRunID,
			item.ID(),
			item.Status(),
		)
	}
}

func (terminalization parkedRunTerminalization) terminalConversationMessages() ([]corechat.Message, error) {
	resultText := "tool call canceled before completion"
	if terminalization.outcome == rundomain.OutcomeLost {
		resultText = "tool result unavailable because execution state was lost"
	} else if terminalization.detail != "" {
		resultText += ": " + terminalization.detail
	}
	history, err := conversation.New(terminalization.snapshot.Messages)
	if err != nil {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q conversation: %w",
			terminalization.rootRunID,
			err,
		)
	}
	_, appended, err := history.CloseOpenToolCalls(resultText)
	if err != nil {
		return nil, fmt.Errorf(
			"sessions: close parked Run tree %q Tool context: %w",
			terminalization.rootRunID,
			err,
		)
	}
	return appended, nil
}

func terminalGoalRun(root rundomain.Run) goal.RunRecord {
	outcome, _ := root.Outcome()
	record := goal.RunRecord{
		SessionID:     root.SessionID(),
		IncarnationID: root.GoalIncarnationID(),
		RunID:         root.ID(),
		Outcome:       outcome,
		Steps:         root.Metrics().Steps(),
		CompletedAt:   root.FinishedAt(),
	}
	if usage, reported := root.Metrics().Usage(); reported && usage.Total.CostUSD != nil {
		record.CostUSD = *usage.Total.CostUSD
	}
	return record
}
