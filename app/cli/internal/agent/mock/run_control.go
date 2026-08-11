package mock

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (r *Runtime) StartRun(ctx context.Context, in agent.StartRun) (agent.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return agent.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.SegmentStream{}, err
	}
	build := r.Script
	if build == nil {
		build = DefaultScript
	}
	script, err := buildScriptSafely(build, strings.TrimSpace(in.Message.Text))
	if err != nil {
		return agent.SegmentStream{}, fmt.Errorf("mock: build script: %w", err)
	}

	r.mu.Lock()
	session := r.sessions[in.SessionID]
	if session == nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("%w: %s", agent.ErrSessionNotFound, in.SessionID)
	}
	if session.active != "" {
		r.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("%w: %s", agent.ErrSessionHasActiveRun, in.SessionID)
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, err
	}
	r.next++
	run := &runState{
		id: fmt.Sprintf("run_mock_%d", r.next), sessionID: in.SessionID,
		provider: in.Options.Provider, model: in.Options.Model, limits: in.Options.Limits, status: agent.RunStatusRunning,
		segments: make(map[string]*segmentState), script: script, answers: make(map[string]agent.Answer), cancel: make(chan struct{}),
	}
	run.script = namespaceScript(run.script, run.id)
	if run.provider == "" {
		run.provider, run.model = "mock", "balanced"
	}
	segment := r.openSegmentLocked(run)
	r.runs[run.id] = run
	session.active = run.id
	session.runs = append(session.runs, run.id)
	r.setSessionStatusLocked(session, agent.SessionRunning)
	r.emitLocked(run, agent.SegmentStarted{Run: projectRun(run)})
	r.next++
	userItemID := fmt.Sprintf("item_mock_%d", r.next)
	r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
		ID: userItemID, Kind: agent.BlockUser, Text: strings.TrimSpace(in.Message.Text), Attachments: slices.Clone(in.Message.Attachments),
	}})
	stream := r.bindSegmentLocked(ctx, run, segment, 0, 0, userItemID, fault)
	r.mu.Unlock()

	go r.play(run, run.script.Prelude, run.script.interrupts())
	return stream, nil
}

func (r *Runtime) ResumeRun(ctx context.Context, in agent.ResumeRun) (agent.SegmentStream, error) {
	if err := in.Validate(); err != nil {
		return agent.SegmentStream{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.SegmentStream{}, err
	}

	r.mu.Lock()
	run := r.runs[in.RunID]
	if run == nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if err := validateResumeSet(run, in.Answers); err != nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, err
	}
	answers := cloneAnswers(in.Answers)
	allAnswers, err := completeScriptAnswers(run, answers)
	if err != nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, err
	}
	script := run.script
	r.mu.Unlock()

	steps, err := continueSafely(script, allAnswers)
	if err != nil {
		return agent.SegmentStream{}, fmt.Errorf("mock: continue script: %w", err)
	}

	r.mu.Lock()
	if run.status != agent.RunStatusWaiting {
		r.mu.Unlock()
		return agent.SegmentStream{}, fmt.Errorf("%w: run %s", agent.ErrInterruptNotOpen, run.id)
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		r.mu.Unlock()
		return agent.SegmentStream{}, err
	}
	for _, response := range answers {
		run.answers[response.ItemID] = agent.CloneAnswer(response.Answer)
		if approval := findApproval(run.interactions, response.ItemID); approval != nil {
			if answer, ok := response.Answer.(agent.ApprovalAnswer); ok && answer.Remember != agent.RememberNone {
				r.rememberApprovalLocked(run, *approval, answer)
			}
		}
	}
	run.interactions = nil
	run.status = agent.RunStatusRunning
	segment := r.openSegmentLocked(run)
	session := r.sessions[run.sessionID]
	r.setSessionStatusLocked(session, agent.SessionRunning)
	r.emitLocked(run, agent.SegmentStarted{Run: projectRun(run)})
	userItemID := ""
	if in.Message != nil {
		r.next++
		userItemID = fmt.Sprintf("item_mock_%d", r.next)
		r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{ID: userItemID, Kind: agent.BlockUser, Text: strings.TrimSpace(in.Message.Text), Attachments: slices.Clone(in.Message.Attachments)}})
	}
	r.completeApprovalItemsLocked(run, answers)
	stream := r.bindSegmentLocked(ctx, run, segment, 0, 0, userItemID, fault)
	r.mu.Unlock()

	go r.play(run, steps, false)
	return stream, nil
}

func completeScriptAnswers(run *runState, provided []agent.InterruptAnswer) ([]agent.InterruptAnswer, error) {
	byID := make(map[string]agent.Answer, len(run.answers)+len(provided))
	for id, answer := range run.answers {
		byID[id] = answer
	}
	for _, answer := range provided {
		byID[answer.ItemID] = answer.Answer
	}
	complete := make([]agent.InterruptAnswer, 0, len(run.script.Interactions))
	for _, interaction := range run.script.Interactions {
		id := agent.InteractionItemID(interaction)
		answer, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("mock: script interrupt %s has no answer", id)
		}
		complete = append(complete, agent.InterruptAnswer{ItemID: id, Answer: agent.CloneAnswer(answer)})
	}
	return complete, nil
}

func validateResumeSet(run *runState, answers []agent.InterruptAnswer) error {
	if run.status != agent.RunStatusWaiting {
		return fmt.Errorf("%w: run %s", agent.ErrInterruptNotOpen, run.id)
	}
	if len(answers) != len(run.interactions) {
		return fmt.Errorf("mock: resume answers %d interrupts; waiting set has %d", len(answers), len(run.interactions))
	}
	byID := make(map[string]agent.Answer, len(answers))
	for _, answer := range answers {
		byID[answer.ItemID] = answer.Answer
	}
	for _, interaction := range run.interactions {
		id := agent.InteractionItemID(interaction)
		answer, ok := byID[id]
		if !ok {
			return fmt.Errorf("mock: waiting interrupt %s has no answer", id)
		}
		if err := agent.ValidateAnswer(interaction, answer); err != nil {
			return fmt.Errorf("mock: interrupt %s: %w", id, err)
		}
	}
	return nil
}

func findApproval(interactions []agent.Interaction, id string) *agent.Approval {
	for _, interaction := range interactions {
		if approval, ok := interaction.(agent.Approval); ok && approval.ItemID == id {
			return &approval
		}
	}
	return nil
}

func cloneAnswers(answers []agent.InterruptAnswer) []agent.InterruptAnswer {
	out := slices.Clone(answers)
	for i := range out {
		out[i].Answer = agent.CloneAnswer(out[i].Answer)
	}
	return out
}

func (r *Runtime) CancelRun(ctx context.Context, in agent.CancelRun) (agent.Run, error) {
	if err := in.Validate(); err != nil {
		return agent.Run{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.Run{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return agent.Run{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if run.status == agent.RunStatusFinished {
		return projectRun(run), nil
	}
	run.cancelOnce.Do(func() { close(run.cancel) })
	r.finishLocked(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCanceled, Detail: strings.TrimSpace(in.Reason)}})
	return projectRun(run), nil
}
