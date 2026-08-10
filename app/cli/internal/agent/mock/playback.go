package mock

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
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
	if run.status != agent.RunActive {
		r.mu.Unlock()
		return
	}
	var remembered *agent.ApprovalAnswer
	if approval, ok := run.script.Interaction.(agent.Approval); ok {
		if answer, matched := r.rememberedAnswerLocked(run, approval); matched {
			remembered = &answer
		}
	}
	if remembered != nil {
		approval := run.script.Interaction.(agent.Approval)
		r.mu.Unlock()
		steps, err := continueSafely(run.script, *remembered)
		if err != nil {
			r.finish(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeFailed, Error: "mock continuation: " + err.Error()}})
			return
		}
		r.mu.Lock()
		if run.status != agent.RunActive {
			r.mu.Unlock()
			return
		}
		r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
			ID: run.id + "_approval_rule", Kind: agent.BlockNotice,
			Text: "Applied remembered approval rule: " + approvalRuleKey(approval),
		}})
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = agent.RunWaiting
	run.interaction = agent.CloneInteraction(run.script.Interaction)
	r.emitLocked(run, agent.RunInterrupted{Interaction: agent.CloneInteraction(run.interaction)})
	r.mu.Unlock()
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
	if run.status != agent.RunActive {
		return false
	}
	r.emitLocked(run, event)
	return true
}

func (r *Runtime) emitLocked(run *runState, event agent.Event) {
	session := r.sessions[run.sessionID]
	r.next++
	envelope := agent.Envelope{
		ID: fmt.Sprintf("evt_mock_%d", r.next), Cursor: agent.Cursor(len(session.events) + 1),
		RunID: run.id, SessionID: run.sessionID, At: r.now(), Event: cloneEvent(event),
	}
	session.events = append(session.events, envelope)
	session.meta.UpdatedAt = envelope.At
	session.meta.Revision++
	if _, ok := event.(agent.RunFinished); ok {
		run.status = agent.RunComplete
		session.active = ""
	}
	close(session.changed)
	session.changed = make(chan struct{})
}

func (r *Runtime) finish(run *runState, event agent.RunFinished) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked(run, event)
}

func (r *Runtime) finishLocked(run *runState, event agent.RunFinished) {
	run.finishOnce.Do(func() { r.emitLocked(run, event) })
}
