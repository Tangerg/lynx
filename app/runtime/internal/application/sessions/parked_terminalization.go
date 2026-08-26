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

func (p parkedRunTerminalization) build() (TerminalPlan, rundomain.Run, error) {
	if err := p.pending.Validate(); err != nil {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: invalid Pending: %w",
			p.rootRunID,
			err,
		)
	}
	if p.outcome != rundomain.OutcomeCanceled &&
		p.outcome != rundomain.OutcomeLost {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: unsupported outcome %s",
			p.rootRunID,
			p.outcome,
		)
	}
	runsByID, rootAdmission, err := p.indexRuns()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	conversationMessages, err := p.terminalConversationMessages()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	terminalRuns, err := p.terminalRuns(
		runsByID,
		rootAdmission,
		len(p.snapshot.Messages)+len(conversationMessages),
	)
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	rootRun := terminalRuns[len(terminalRuns)-1]
	if rootRun.ID() != p.rootRunID || rootRun.GoalIncarnationID() != p.pending.GoalIncarnationID {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root admission differs from Pending",
			p.rootRunID,
		)
	}
	items, err := p.terminalItems()
	if err != nil {
		return TerminalPlan{}, rundomain.Run{}, err
	}
	root, ok := p.pending.RootContinuation()
	if !ok {
		return TerminalPlan{}, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root continuation is missing",
			p.rootRunID,
		)
	}
	plan := TerminalPlan{
		Runs: terminalRuns, Items: items, Messages: conversationMessages,
		CheckpointRootID: root.MemberID, ResumeClaimed: p.resumeClaimed,
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

func (p parkedRunTerminalization) indexRuns() (
	map[string]rundomain.Run,
	rundomain.Run,
	error,
) {
	runsByID := make(map[string]rundomain.Run, len(p.snapshot.Runs))
	for _, run := range p.snapshot.Runs {
		if _, duplicate := runsByID[run.ID()]; duplicate {
			return nil, rundomain.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: duplicate Run %q",
				p.rootRunID,
				run.ID(),
			)
		}
		runsByID[run.ID()] = run
	}
	rootAdmission, found := runsByID[p.rootRunID]
	if !found || !rootAdmission.Lineage().IsRoot() {
		return nil, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: root Run is missing",
			p.rootRunID,
		)
	}
	if !p.pending.Capabilities.Equal(rootAdmission.Capabilities()) {
		return nil, rundomain.Run{}, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: Pending run capabilities differ from root Run admission",
			p.rootRunID,
		)
	}
	pendingRunIDs := make(map[string]struct{}, len(p.pending.Continuations))
	for _, continuation := range p.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
	}
	for _, run := range p.snapshot.Runs {
		if run.Lineage().TreeRootID(run.ID()) != p.rootRunID || run.State().IsTerminal() {
			continue
		}
		if _, covered := pendingRunIDs[run.ID()]; !covered {
			return nil, rundomain.Run{}, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: non-terminal Run %q has no Pending continuation",
				p.rootRunID,
				run.ID(),
			)
		}
	}
	return runsByID, rootAdmission, nil
}

func (p parkedRunTerminalization) terminalRuns(
	runsByID map[string]rundomain.Run,
	rootAdmission rundomain.Run,
	messageMark int,
) ([]rundomain.Run, error) {
	terminalRuns := make([]rundomain.Run, 0, len(p.pending.Continuations))
	for _, continuation := range p.pending.Continuations {
		run, found := runsByID[continuation.RunID]
		if !found {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q is missing",
				p.rootRunID,
				continuation.RunID,
			)
		}
		if !waitingRunMatchesContinuation(run, continuation, p.sessionID, rootAdmission.Capabilities()) {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: Run %q differs from its continuation",
				p.rootRunID,
				run.ID(),
			)
		}
		var terminal rundomain.Run
		var err error
		if p.outcome == rundomain.OutcomeLost {
			terminal, err = run.RecoverLost(rundomain.Failure{
				Kind:   rundomain.FailureLost,
				Detail: "the parked Run tree's executor checkpoint could not be restored",
			}, p.finishedAt, messageMark)
		} else {
			terminal, err = run.CancelWaiting(
				p.detail,
				p.finishedAt,
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

func (p parkedRunTerminalization) terminalItems() ([]transcript.Item, error) {
	interruptItems := make(map[string]transcript.Interrupt, len(p.pending.Interrupts))
	for _, interruption := range p.pending.Interrupts {
		interruptItems[interruption.ItemID] = interruption
	}
	pendingRunIDs := make(map[string]struct{}, len(p.pending.Continuations))
	drainedItems := make(map[string]runs.DrainedTool)
	for _, continuation := range p.pending.Continuations {
		pendingRunIDs[continuation.RunID] = struct{}{}
		for _, drained := range continuation.DrainedTools {
			drainedItems[drained.ItemID] = drained
		}
	}
	items := make([]transcript.Item, 0, len(interruptItems)+len(drainedItems))
	for _, item := range p.snapshot.Items {
		request, found := interruptItems[item.ID()]
		if !found {
			if drained, drainedFound := drainedItems[item.ID()]; drainedFound {
				settled, changed, err := p.terminalDrainedItem(item, drained)
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
					p.rootRunID,
					item.ID(),
				)
			}
			continue
		}
		if item.SessionID() != p.sessionID || item.RunID() != request.RunID {
			return nil, fmt.Errorf(
				"sessions: terminalize parked Run tree %q: interrupt Item %q is not owned by Run %q",
				p.rootRunID,
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
			if p.outcome == rundomain.OutcomeLost {
				failure = &tool.Failure{
					Kind:   tool.FailureExecution,
					Detail: "tool call abandoned because its run could not be resumed",
				}
			}
			settled, err := item.AbandonToolCall(failure, p.finishedAt)
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
			p.rootRunID,
		)
	}
	if len(drainedItems) != 0 {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: transcript is missing %d drained Tool Items",
			p.rootRunID,
			len(drainedItems),
		)
	}
	return items, nil
}

func (p parkedRunTerminalization) terminalDrainedItem(
	item transcript.Item,
	drained runs.DrainedTool,
) (transcript.Item, bool, error) {
	invocation, present := item.ToolInvocation()
	if item.SessionID() != p.sessionID || item.Kind() != transcript.ToolCall ||
		!present || invocation.Name != drained.Name ||
		invocation.Arguments.Canonical() != drained.Arguments {
		return transcript.Item{}, false, fmt.Errorf(
			"sessions: terminalize parked Run tree %q: drained Tool Item %q differs from its continuation",
			p.rootRunID,
			item.ID(),
		)
	}
	switch item.Status() {
	case transcript.ItemRunning:
		var failure *tool.Failure
		if p.outcome == rundomain.OutcomeLost {
			failure = &tool.Failure{
				Kind:   tool.FailureExecution,
				Detail: "tool call abandoned because its run could not be resumed",
			}
		}
		settled, err := item.AbandonToolCall(failure, p.finishedAt)
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
			p.rootRunID,
			item.ID(),
			item.Status(),
		)
	}
}

func (p parkedRunTerminalization) terminalConversationMessages() ([]corechat.Message, error) {
	resultText := "tool call canceled before completion"
	if p.outcome == rundomain.OutcomeLost {
		resultText = "tool result unavailable because execution state was lost"
	} else if p.detail != "" {
		resultText += ": " + p.detail
	}
	history, err := conversation.New(p.snapshot.Messages)
	if err != nil {
		return nil, fmt.Errorf(
			"sessions: terminalize parked Run tree %q conversation: %w",
			p.rootRunID,
			err,
		)
	}
	_, appended, err := history.CloseOpenToolCalls(resultText)
	if err != nil {
		return nil, fmt.Errorf(
			"sessions: close parked Run tree %q Tool context: %w",
			p.rootRunID,
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
