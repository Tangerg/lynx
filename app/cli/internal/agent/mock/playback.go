package mock

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/failure"
)

func (r *Runtime) play(run *runState, steps []Step, interrupt bool) {
	if !r.playSteps(run, steps) || !interrupt {
		return
	}
	r.park(run)
}

func (r *Runtime) playSteps(run *runState, steps []Step) bool {
	for _, step := range steps {
		if err := r.pause(run, step.Delay); err != nil {
			if errors.Is(err, errCanceled) {
				r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCanceled}})
			}
			return false
		}
		if finished, done := step.Event.(agent.RunFinished); done {
			r.finish(run, finished)
			return false
		}
		if !r.emit(run, step.Event) {
			return false
		}
	}
	return true
}

func (r *Runtime) park(run *runState) {
	r.mu.Lock()
	if run.status != agent.RunStatusRunning {
		r.mu.Unlock()
		return
	}
	if err := r.ensureInterruptItemsLocked(run); err != nil {
		r.mu.Unlock()
		r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeFailed, Problem: &failure.Problem{Type: "mock_interrupt_failed", Detail: err.Error()}}})
		return
	}
	resolved, pending := r.resolveRememberedLocked(run, run.script.Interactions)
	for _, answer := range resolved {
		run.answers[answer.ItemID] = agent.CloneAnswer(answer.Answer)
	}
	if len(resolved) != 0 {
		r.completeApprovalItemsLocked(run, resolved)
		r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
			ID: run.id + "_approval_rule", Kind: agent.BlockNotice,
			Text: "Applied remembered approval rules.",
		}})
	}
	if len(pending) == 0 {
		answers, err := completeScriptAnswers(run, nil)
		var steps []Step
		r.mu.Unlock()
		if err == nil {
			steps, err = continueSafely(run.script, answers)
		}
		if err != nil {
			r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeFailed, Problem: &failure.Problem{Type: "mock_continuation_failed", Detail: err.Error()}}})
			return
		}
		r.mu.Lock()
		if run.status != agent.RunStatusRunning {
			r.mu.Unlock()
			return
		}
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = agent.RunStatusWaiting
	run.interactions = agent.CloneInteractions(pending)
	run.usage = run.script.InterruptUsage.Clone()
	r.emitLocked(run, agent.RunInterrupted{Interactions: agent.CloneInteractions(run.interactions), Usage: run.usage})
	run.active = ""
	r.setSessionStatusLocked(r.sessions[run.sessionID], agent.SessionWaiting)
	r.mu.Unlock()
}

func (r *Runtime) ensureInterruptItemsLocked(run *runState) error {
	session := r.sessions[run.sessionID]
	for _, interaction := range run.script.Interactions {
		itemID := agent.InteractionItemID(interaction)
		if block, exists := durableBlock(session, run.id, itemID); exists {
			switch interaction.(type) {
			case agent.Approval:
				if block.Kind != agent.BlockTool || block.Status != agent.BlockStatusRunning {
					return fmt.Errorf("approval item %s is not a running tool", itemID)
				}
			case agent.Question:
				if block.Kind != agent.BlockQuestion || block.Status != agent.BlockStatusCompleted {
					return fmt.Errorf("question item %s is not a completed question", itemID)
				}
			}
			continue
		}
		switch item := interaction.(type) {
		case agent.Approval:
			r.emitLocked(run, agent.BlockStarted{Block: agent.Block{
				ID: item.ItemID, Kind: agent.BlockTool, Tool: cloneTool(item.Tool),
			}})
		case agent.Question:
			question := item.Clone()
			r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
				ID: item.ItemID, Kind: agent.BlockQuestion, Question: &question,
			}})
		}
	}
	return nil
}

func (r *Runtime) completeApprovalItemsLocked(run *runState, answers []agent.InterruptAnswer) {
	for _, response := range answers {
		approval := findApproval(run.script.Interactions, response.ItemID)
		answer, ok := response.Answer.(agent.ApprovalAnswer)
		if approval == nil || !ok {
			continue
		}
		tool := cloneTool(approval.Tool)
		tool.Status = agent.ToolOK
		if answer.ArgumentOverride != nil {
			tool.ArgumentsJSON = answer.ArgumentOverride.JSON()
		}
		if answer.Decision == agent.ApprovalDeny {
			tool.Status = agent.ToolError
			tool.Output = strings.TrimSpace(answer.Reason)
			if tool.Output == "" {
				tool.Output = "tool call denied by user"
			}
		}
		r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{ID: approval.ItemID, Kind: agent.BlockTool, Tool: tool}})
	}
}

func durableBlock(session *sessionState, runID, itemID string) (agent.Block, bool) {
	for _, item := range session.items {
		if item.runID == runID && item.block.ID == itemID {
			return item.block.Clone(), true
		}
	}
	return agent.Block{}, false
}

func cloneTool(tool *agent.ToolCall) *agent.ToolCall {
	if tool == nil {
		return nil
	}
	cloned := tool.Clone()
	return &cloned
}

func (r *Runtime) pause(run *runState, delay time.Duration) error {
	if r.Instant || delay <= 0 {
		select {
		case <-run.cancel:
			return errCanceled
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-run.cancel:
		return errCanceled
	}
}

func (r *Runtime) emit(run *runState, event agent.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != agent.RunStatusRunning {
		return false
	}
	r.emitLocked(run, event)
	return true
}

func (r *Runtime) emitLocked(run *runState, event agent.Event) {
	segment := run.segments[run.active]
	if segment == nil {
		panic("mock: active run has no segment")
	}
	session := r.sessions[run.sessionID]
	switch item := event.(type) {
	case agent.BlockStarted:
		item.Block.RunID = run.id
		item.Block.Status = agent.BlockStatusRunning
		event = item
	case agent.BlockCompleted:
		item.Block.RunID = run.id
		if item.Block.Status != agent.BlockStatusIncomplete {
			item.Block.Status = completedBlockStatus(item.Block)
		}
		event = item
	case agent.PlanChanged:
		session.planRevision++
		item.Revision = session.planRevision
		event = item
	}
	r.next++
	envelope := agent.RunEvent{
		EventID: fmt.Sprintf("evt_mock_%d", r.next), RunID: run.id,
		SegmentID: segment.id, At: r.now(), Event: agent.CloneEvent(event),
	}
	segment.events = append(segment.events, envelope)
	session.meta.UpdatedAt = envelope.At
	session.meta.Revision++
	switch item := event.(type) {
	case agent.BlockStarted:
		if item.Block.Kind == agent.BlockTool {
			persistBlock(session, run.id, item.Block)
		}
	case agent.BlockCompleted:
		persistBlock(session, run.id, item.Block)
	case agent.PlanChanged:
		session.plan = slices.Clone(item.Items)
	case agent.RunInterrupted:
		r.closeSegmentLocked(segment)
	case agent.RunFinished:
		r.closeSegmentLocked(segment)
	}
	close(segment.changed)
	segment.changed = make(chan struct{})
}

func persistBlock(session *sessionState, runID string, block agent.Block) {
	for i := range session.items {
		if session.items[i].runID == runID && session.items[i].block.ID == block.ID {
			session.items[i] = durableItem{runID: runID, block: block.Clone()}
			return
		}
	}
	session.items = append(session.items, durableItem{runID: runID, block: block.Clone()})
}

func (r *Runtime) closeSegmentLocked(segment *segmentState) {
	segment.closed = true
}

func (r *Runtime) finish(run *runState, event agent.RunFinished) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked(run, event)
}

func (r *Runtime) finishLocked(run *runState, event agent.RunFinished) {
	run.finishOnce.Do(func() {
		run.outcome = event.Outcome
		run.usage = event.Usage.Clone()
		r.settleRunningItemsLocked(run, event.Outcome)
		if run.active != "" {
			r.emitLocked(run, event)
		}
		run.status = agent.RunStatusFinished
		run.active = ""
		run.interactions = nil
		session := r.sessions[run.sessionID]
		if session.planAtRun == nil {
			session.planAtRun = make(map[string][]agent.PlanItem)
		}
		session.planAtRun[run.id] = slices.Clone(session.plan)
		session.active = ""
		r.setSessionStatusLocked(session, agent.SessionIdle)
	})
}

func (r *Runtime) settleRunningItemsLocked(run *runState, outcome agent.Outcome) {
	session := r.sessions[run.sessionID]
	var unsettled []agent.Block
	for _, item := range session.items {
		if item.runID == run.id && item.block.Status == agent.BlockStatusRunning {
			block := item.block.Clone()
			block.Status = agent.BlockStatusIncomplete
			if block.Tool != nil {
				block.Tool.Status = agent.ToolError
				if outcome.Status == agent.OutcomeCanceled {
					block.Tool.Status = agent.ToolCanceled
				}
			}
			unsettled = append(unsettled, block)
		}
	}
	for _, block := range unsettled {
		if run.active != "" {
			r.emitLocked(run, agent.BlockCompleted{Block: block})
		} else {
			persistBlock(session, run.id, block)
		}
	}
}

func (r *Runtime) setSessionStatusLocked(session *sessionState, status agent.SessionStatus) {
	if session.meta.Status == status {
		return
	}
	session.meta.Status = status
	session.meta.Revision++
	session.meta.UpdatedAt = r.now()
}
