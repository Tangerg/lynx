package mock

import (
	"context"
	"fmt"
	"maps"
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
	r.runOrder = append(r.runOrder, run.id)
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
	prepared, err := r.prepareResumeLocked(in)
	r.mu.Unlock()
	if err != nil {
		return agent.SegmentStream{}, err
	}
	steps, err := prepared.continueScript()
	if err != nil {
		return agent.SegmentStream{}, err
	}

	r.mu.Lock()
	stream, err := r.activateResumeLocked(ctx, in.Message, prepared)
	r.mu.Unlock()
	if err != nil {
		return agent.SegmentStream{}, err
	}

	go r.play(prepared.run, steps, false)
	return stream, nil
}

type resumePreparation struct {
	run        *runState
	answers    []agent.InterruptAnswer
	allAnswers []agent.InterruptAnswer
	script     Script
}

func (prepared resumePreparation) continueScript() ([]Step, error) {
	steps, err := continueSafely(prepared.script, prepared.allAnswers)
	if err != nil {
		return nil, fmt.Errorf("mock: continue script: %w", err)
	}
	return steps, nil
}

func (r *Runtime) prepareResumeLocked(in agent.ResumeRun) (resumePreparation, error) {
	run := r.runs[in.RunID]
	if run == nil {
		return resumePreparation{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if err := validateResumeSet(run, in.Answers); err != nil {
		return resumePreparation{}, err
	}
	answers := cloneAnswers(in.Answers)
	allAnswers, err := completeScriptAnswers(run, answers)
	if err != nil {
		return resumePreparation{}, err
	}
	return resumePreparation{run: run, answers: answers, allAnswers: allAnswers, script: run.script}, nil
}

func (r *Runtime) activateResumeLocked(ctx context.Context, message *agent.Message, prepared resumePreparation) (agent.SegmentStream, error) {
	run := prepared.run
	if run.status != agent.RunStatusWaiting {
		return agent.SegmentStream{}, fmt.Errorf("%w: run %s", agent.ErrInterruptNotOpen, run.id)
	}
	fault, err := r.takeFaultLocked()
	if err != nil {
		return agent.SegmentStream{}, err
	}
	r.recordAnswersLocked(run, prepared.answers)
	run.interactions = nil
	run.status = agent.RunStatusRunning
	segment := r.openSegmentLocked(run)
	r.setSessionStatusLocked(r.sessions[run.sessionID], agent.SessionRunning)
	r.emitLocked(run, agent.SegmentStarted{Run: projectRun(run)})
	userItemID := r.emitResumeMessageLocked(run, message)
	r.completeApprovalItemsLocked(run, prepared.answers)
	return r.bindSegmentLocked(ctx, run, segment, 0, 0, userItemID, fault), nil
}

func (r *Runtime) recordAnswersLocked(run *runState, answers []agent.InterruptAnswer) {
	for _, response := range answers {
		run.answers[response.ItemID] = agent.CloneAnswer(response.Answer)
		approval := findApproval(run.interactions, response.ItemID)
		answer, ok := response.Answer.(agent.ApprovalAnswer)
		if approval != nil && ok && answer.Remember != agent.RememberNone {
			r.rememberApprovalLocked(run, *approval, answer)
		}
	}
}

func (r *Runtime) emitResumeMessageLocked(run *runState, message *agent.Message) string {
	if message == nil {
		return ""
	}
	r.next++
	itemID := fmt.Sprintf("item_mock_%d", r.next)
	r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
		ID: itemID, Kind: agent.BlockUser, Text: strings.TrimSpace(message.Text), Attachments: slices.Clone(message.Attachments),
	}})
	return itemID
}

func completeScriptAnswers(run *runState, provided []agent.InterruptAnswer) ([]agent.InterruptAnswer, error) {
	byID := make(map[string]agent.Answer, len(run.answers)+len(provided))
	maps.Copy(byID, run.answers)
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

func (r *Runtime) CancelRun(ctx context.Context, in agent.CancelRun) (agent.RunCancellation, error) {
	if err := in.Validate(); err != nil {
		return agent.RunCancellation{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return agent.RunCancellation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return agent.RunCancellation{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if run.status == agent.RunStatusFinished {
		return agent.RunCancellation{}, fmt.Errorf("%w: %s", agent.ErrRunFinished, run.id)
	}
	run.cancelOnce.Do(func() { close(run.cancel) })
	r.finishLocked(run, agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCanceled, Detail: strings.TrimSpace(in.Reason)}})
	projected := projectRun(run)
	return agent.RunCancellation{Canceled: projected, Root: projected.Clone()}, nil
}

func (r *Runtime) SteerRun(ctx context.Context, in agent.SteerRun) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[in.RunID]
	if run == nil {
		return fmt.Errorf("%w: %s", agent.ErrRunNotFound, in.RunID)
	}
	if run.status != agent.RunStatusRunning || run.active != in.SegmentID {
		return fmt.Errorf("%w: run %s is not executing segment %s", agent.ErrStaleSegment, in.RunID, in.SegmentID)
	}
	r.next++
	r.emitLocked(run, agent.BlockCompleted{Block: agent.Block{
		ID: fmt.Sprintf("item_mock_%d", r.next), Kind: agent.BlockUser,
		Text: strings.TrimSpace(in.Message.Text), Attachments: slices.Clone(in.Message.Attachments),
	}})
	return nil
}
