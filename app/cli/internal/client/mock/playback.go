package mock

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
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
				r.finish(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
			}
			return false
		}
		if finished, done := step.Event.(client.RunFinished); done {
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
	if run.status != client.RunActive {
		r.mu.Unlock()
		return
	}
	var remembered *client.ApprovalAnswer
	if approval, ok := run.script.Interaction.(client.Approval); ok {
		if answer, matched := r.rememberedAnswerLocked(run, approval); matched {
			remembered = &answer
		}
	}
	if remembered != nil {
		approval := run.script.Interaction.(client.Approval)
		r.mu.Unlock()
		steps, err := continueSafely(run.script, *remembered)
		if err != nil {
			r.finish(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeFailed, Error: "mock continuation: " + err.Error()}})
			return
		}
		r.mu.Lock()
		if run.status != client.RunActive {
			r.mu.Unlock()
			return
		}
		r.emitLocked(run, client.BlockCompleted{Block: client.Block{
			ID: run.id + "_approval_rule", Kind: client.BlockNotice,
			Text: "Applied remembered approval rule: " + approvalRuleKey(approval),
		}})
		r.mu.Unlock()
		r.play(run, steps, false)
		return
	}
	run.status = client.RunWaiting
	run.interaction = client.CloneInteraction(run.script.Interaction)
	r.emitLocked(run, client.RunInterrupted{Interaction: client.CloneInteraction(run.interaction)})
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

func (r *Runtime) emit(run *runState, event client.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run.status != client.RunActive {
		return false
	}
	r.emitLocked(run, event)
	return true
}

func (r *Runtime) emitLocked(run *runState, event client.Event) {
	session := r.sessions[run.sessionID]
	r.next++
	envelope := client.Envelope{
		ID: fmt.Sprintf("evt_mock_%d", r.next), Cursor: client.Cursor(len(session.events) + 1),
		RunID: run.id, SessionID: run.sessionID, At: r.now(), Event: cloneEvent(event),
	}
	session.events = append(session.events, envelope)
	session.meta.UpdatedAt = envelope.At
	session.meta.Revision++
	if _, ok := event.(client.RunFinished); ok {
		run.status = client.RunComplete
		session.active = ""
	}
	close(session.changed)
	session.changed = make(chan struct{})
}

func (r *Runtime) finish(run *runState, event client.RunFinished) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked(run, event)
}

func (r *Runtime) finishLocked(run *runState, event client.RunFinished) {
	run.finishOnce.Do(func() { r.emitLocked(run, event) })
}
