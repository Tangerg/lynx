package terminal

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type approvalPane struct {
	theme         kit.Theme
	glyphs        kit.Glyphs
	title         string
	detail        *kit.Paragraph
	preview       headless.Transcript
	scroll        headless.Scroll
	view          kit.Transcript
	form          *kit.Form
	presentedForm headless.Snapshot[*kit.Form]
}

type approvalDecisionDraft struct {
	choice approvalAction
	reason string
}

func (draft approvalDecisionDraft) answer(action approvalAction, override *agent.ToolArgumentOverride) (agent.ApprovalAnswer, bool) {
	decision, ok := action.Answer()
	if !ok {
		return agent.ApprovalAnswer{}, false
	}
	if decision.Decision == agent.ApprovalDeny {
		decision.Reason = strings.TrimSpace(draft.reason)
		if decision.Reason == "" {
			decision.Reason = "denied by the user in the terminal"
		}
	} else {
		decision.ArgumentOverride = override.Clone()
	}
	return decision, true
}

func (p *approvalPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	form := p.form
	p.presentedForm.Stage(frame, form)
	if width <= 0 || height <= 0 || form == nil {
		return
	}
	formRows := min(form.Measure(width), max(height-1, 0))
	detailRows := min(p.detail.Measure(width), min(4, max(height-formRows-1, 0)))
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(detailRows)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(formRows)},
	}))
	kit.Label{Text: p.title, Style: p.theme.Strong, Ellipsis: "…"}.Draw(rows[0].View)
	p.detail.Draw(rows[1].View)
	p.view.Draw(rows[2])
	form.Draw(rows[3])
}

func (p *approvalPane) Handle(event input.Event) bool {
	if form := p.presentedForm.Value(); form != nil && form.Handle(event) {
		return true
	}
	return p.view.Handle(event)
}

func (p *approvalPane) Focus(has bool) {
	if p.form != nil {
		p.form.Focus(has)
	}
}

func (a *app) buildApprovalDialog(theme kit.Theme, glyphs kit.Glyphs) {
	a.approvalPane = approvalPane{
		theme: theme, glyphs: glyphs, detail: kit.NewParagraph("", theme.Text),
	}
	a.approvalPane.scroll.Wheel(a.loop.Environment().Wheel())
	a.approvalPane.view = kit.Transcript{
		Content: &a.approvalPane.preview, Scroll: &a.approvalPane.scroll,
		Theme: theme, Glyphs: glyphs,
	}
	a.setApprovalForm(approvalAllowOnce)
	a.approvalDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Tool approval", Body: &a.approvalPane,
		Where: layout.Placement{Width: 88, Height: 24},
	})
}

func (a *app) setApprovalForm(initial approvalAction) {
	review := a.interactionReview
	approval := a.approval
	draft := a.approvalDraft
	if draft == nil {
		draft = &approvalDecisionDraft{}
		a.approvalDraft = draft
	}
	rememberable := a.approval == nil || a.approval.Rememberable
	draft.choice = initial.Normalize(rememberable)
	choice := &headless.Select[approvalAction]{
		Label: "How should lyra proceed?", Value: headless.Bind(&draft.choice), Rows: 3,
	}
	choice.SetOptions(approvalOptions(rememberable))
	reason := &headless.Text{
		Label: "Denial feedback (optional)", Placeholder: "Explain what should change before retrying",
		Value: headless.Bind(&draft.reason),
	}
	reason.Editor().Clipboard = a.loop.Clipboard()
	keys := headless.DefaultFormKeys()
	a.approvalForm = headless.NewForm(choice, reason)
	a.approvalForm.Keys = keys
	a.approvalForm.Done = func() {
		if a.interactionReview == review && a.approval == approval && a.approvalDraft == draft {
			a.answerApproval(draft.choice)
		}
	}
	a.approvalForm.GaveUp = func() {
		if a.interactionReview == review && a.approval == approval && a.approvalDraft == draft {
			a.backOrCancelApproval()
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.approvalPane.theme, Glyphs: a.approvalPane.glyphs, Controller: a.approvalForm,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.approvalPane.form = dressed
}

func (a *app) openApproval(approval agent.Approval) {
	cloned := approval.Clone()
	a.approval = &cloned
	a.approvalDraft = &approvalDecisionDraft{}
	a.approvalArguments = editableApprovalArguments(approval.Tool)
	a.approvalOverride = nil
	initial := defaultApprovalAction(a.settings.Approval.Remember.Scope())
	if answer, ok := a.interactionReview.CurrentAnswer().(agent.ApprovalAnswer); ok {
		initial = approvalActionFromAnswer(answer)
		a.approvalDraft.reason = answer.Reason
		if answer.ArgumentOverride != nil {
			a.approvalOverride = answer.ArgumentOverride.Clone()
			a.approvalArguments = formatToolArguments(a.approvalOverride.JSON())
		}
	}
	a.setApprovalForm(initial)
	a.approvalPane.title = approval.Title
	call := approval.Tool.Clone()
	if strings.TrimSpace(call.Diff) == "" {
		call.Diff = approval.Diff
	}
	presentation, err := selectToolPresentation(a.registry.Values(ToolPresenters), call)
	if err != nil {
		presentation = ToolPresentation{
			Label:    toolLabel(call),
			Sections: []ToolSection{{Title: "Presentation error", Style: toolSectionParagraph, Text: err.Error()}},
		}
	}
	a.approvalSections = slices.Clone(presentation.Sections)
	details := []string{a.interactionReview.SubmissionFailure(), approval.Detail, presentation.Label}
	if approval.Risk != "" {
		details = append(details, "risk: "+string(approval.Risk))
	}
	if approval.RuleHint != "" {
		details = append(details, "rule: "+approval.RuleHint)
	}
	a.approvalPane.detail.SetText([]text.Line{text.Of(strings.Join(nonEmptyStrings(details), "\n"), a.approvalPane.theme.Text)})
	a.setApprovalPreview(a.approvalPreviewSections())
	a.approvalDialog.Controller().SetDescription(approval.Title)
	a.approvalDialog.Show()
}

func (a *app) setApprovalPreview(sections []ToolSection) {
	a.approvalPane.preview = headless.Transcript{}
	a.approvalPane.view.Content = &a.approvalPane.preview
	presentation := BlockPresentation{
		Theme: a.approvalPane.theme, Glyphs: a.approvalPane.glyphs, Syntax: a.syntax,
	}
	blockCount := 0
	for _, section := range sections {
		for _, block := range renderToolSections(presentation, []ToolSection{section}, false) {
			id := a.approvalPane.preview.Append(newReaderSectionBlock(a.approvalPane.theme, section.Title, block))
			a.approvalPane.preview.Finish(id)
			blockCount++
		}
	}
	if blockCount == 0 {
		id := a.approvalPane.preview.Append(&kit.Message{Theme: a.approvalPane.theme, Body: "This request has no additional preview."})
		a.approvalPane.preview.Finish(id)
	}
	a.approvalPane.scroll = headless.Scroll{}
	a.approvalPane.scroll.Wheel(a.loop.Environment().Wheel())
	a.approvalPane.scroll.ToTop()
	a.approvalPane.view.Scroll = &a.approvalPane.scroll
}

func (a *app) openInteractions(interactions []agent.Interaction) {
	if a.interactionReview != nil {
		a.fail(errors.New("runtime opened interactions while another set is active"))
		return
	}
	review, err := newInteractionReview(interactions)
	if err != nil {
		a.fail(fmt.Errorf("runtime interactions: %w", err))
		return
	}
	a.interactionReview = review
	a.openCurrentInteraction()
	a.raiseAttention(interactionAttention(interactions))
}

func (a *app) openCurrentInteraction() {
	if a.interactionReview == nil {
		return
	}
	if a.interactionReview.Reviewing() {
		a.openInteractionSummary()
		return
	}
	interaction, ok := a.interactionReview.Current()
	if !ok {
		a.fail(errors.New("interaction review has no current item"))
		return
	}
	switch item := interaction.(type) {
	case agent.Approval:
		a.openApproval(item)
	case agent.Question:
		a.openQuestion(item)
	default:
		a.fail(errors.New("runtime returned an unknown interaction"))
	}
}

func (a *app) answerApproval(action approvalAction) {
	approval := a.approval
	draft := a.approvalDraft
	if approval == nil || draft == nil {
		return
	}
	if action == approvalEditArgs {
		a.openApprovalArgumentEditor()
		return
	}
	decision, ok := draft.answer(action, a.approvalOverride)
	if !ok {
		a.fail(fmt.Errorf("approval action %q is unsupported", action))
		return
	}
	a.submitApproval(decision)
}

func (a *app) submitApproval(decision agent.ApprovalAnswer) {
	if a.interactionReview == nil {
		return
	}
	if err := a.interactionReview.Record(decision); err != nil {
		a.fail(fmt.Errorf("record approval: %w", err))
		return
	}
	a.clearApprovalProjection()
	a.approvalDialog.Dismiss()
	a.advanceInteractionReview()
}

func (a *app) backOrCancelApproval() {
	if a.backInteraction() {
		return
	}
	a.answerApproval(approvalDenyOnce)
}

func (a *app) advanceInteractionReview() {
	if a.interactionReview == nil {
		return
	}
	if a.interactionReview.Advance() || a.interactionReview.Reviewing() {
		a.openCurrentInteraction()
		return
	}
	a.resumeInteractions()
}

func (a *app) backInteraction() bool {
	if a.interactionReview == nil || !a.interactionReview.Back() {
		return false
	}
	if a.approval != nil {
		a.clearApprovalProjection()
		a.approvalDialog.Dismiss()
	}
	if a.questionnaire != nil {
		a.questionnaire = nil
		a.questionDialog.Dismiss()
		a.questionDialog = nil
	}
	if a.reviewDialog != nil {
		a.reviewDialog.Dismiss()
		a.reviewDialog = nil
	}
	a.openCurrentInteraction()
	return true
}

func (a *app) resumeInteractions() {
	if a.interactionReview == nil {
		return
	}
	answers, err := a.interactionReview.Responses()
	if err != nil {
		a.fail(fmt.Errorf("commit interaction review: %w", err))
		return
	}
	runID := a.conversation.RunID()
	review := a.interactionReview
	commandID, err := agent.NewCommandID()
	if err != nil {
		a.fail(err)
		return
	}
	command := agent.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	replay := commandReplayGuard(a.runtimeProfile)
	if a.workbench != nil {
		pending := workbench.PendingResume{
			Command: command.Clone(), Interactions: review.Items(), Replay: replay,
		}
		if err := a.workbench.StagePendingResume(a.session.ID, pending); err != nil {
			failure := fmt.Errorf("resume blocked: save interaction decisions: %w", err)
			review.ReportSubmissionFailure(failure)
			a.message(failure.Error())
			a.status.note("resume blocked · review preserved")
			if reopenErr := a.reopenCompletedInteractionReview(review); reopenErr != nil {
				a.fail(errors.Join(failure, reopenErr))
			}
			return
		}
	}
	a.deliverInteractionResume(review, command, replay)
}

// reopenCompletedInteractionReview restores the UI owner of a completed HITL
// draft when the draft could not be durably staged or its delivery was
// definitively refused. A multi-item draft returns to its review summary; a
// single-item draft returns to the answered interaction so the user can retry
// or revise it.
func (a *app) reopenCompletedInteractionReview(review *interactionReview) error {
	if review == nil || a.interactionReview != review {
		return errors.New("completed interaction review is no longer active")
	}
	if !review.completed() {
		return errors.New("interaction review is not complete")
	}
	if review.Reviewing() {
		a.openInteractionSummary()
		return nil
	}
	if !review.Back() {
		return errors.New("interaction review cannot return to its submitted answer")
	}
	a.openCurrentInteraction()
	return nil
}

func (a *app) deliverInteractionResume(
	review *interactionReview,
	command agent.ResumeRun,
	replay workbench.ReplayGuard,
) {
	a.status.active("resuming")
	a.syncAnimation()
	a.followOpening(func(ctx context.Context) (agent.SegmentStream, error) {
		if err := commandReplayAdmission(replay, a.runtimeProfile)(); err != nil {
			return agent.SegmentStream{}, &resumeRunCallError{err: err}
		}
		stream, err := a.runtime.ResumeRun(ctx, command)
		if err != nil {
			if _, accepted := agent.AcceptedMutationReceipt(err); accepted {
				return agent.SegmentStream{}, err
			}
			return agent.SegmentStream{}, &resumeRunCallError{err: err}
		}
		if err := stream.ValidateResume(command.RunID, nil); err != nil {
			return agent.SegmentStream{}, agent.NewAcceptedMutationError(stream, fmt.Errorf("resume run: %w", err))
		}
		return stream, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(agent.SegmentStream) streamOpeningDisposition {
			a.interactionReview = nil
			a.settleAcknowledgedResume(command.CommandID)
			acceptedQuestions, err := a.conversation.RecordAcceptedInteractionAnswers(command.Answers)
			if err == nil {
				err = a.transcript.acceptQuestions(acceptedQuestions)
			}
			if err != nil {
				failure := fmt.Errorf("project accepted interaction answers: %w", err)
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: command.RunID, Reason: "terminal could not project accepted interaction answers",
				})
				a.fail(failure)
				return rejectOpenedStream
			}
			return followOpenedStream
		},
		rejected: func(failure error) error {
			if _, accepted := agent.AcceptedMutationReceipt(failure); accepted {
				a.interactionReview = nil
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: command.RunID, Reason: "runtime returned an invalid resume receipt",
				})
				return failure
			}
			return a.restoreRejectedInteractionReview(review, command, failure)
		},
	})
}

func (a *app) settleAcknowledgedResume(commandID agent.CommandID) {
	if err := a.retireAcknowledgedResume(commandID); err != nil {
		a.reportWorkbenchIssue(workbenchResumeOutbox, err)
		a.message("could not settle acknowledged interaction decisions: " + err.Error())
		a.retryAuthoringSettlement(
			resumeSettlementOperation,
			func() error { return a.retireAcknowledgedResume(commandID) },
			func() { a.reportWorkbenchIssue(workbenchResumeOutbox, nil) },
		)
		return
	}
	a.reportWorkbenchIssue(workbenchResumeOutbox, nil)
}

func (a *app) retireAcknowledgedResume(commandID agent.CommandID) error {
	if a.workbench == nil {
		return nil
	}
	pending, ok := a.workbench.PendingResume(a.session.ID)
	if !ok {
		return nil
	}
	if pending.Command.CommandID != commandID {
		return errors.New("pending resume command identity changed")
	}
	return a.workbench.AcknowledgePendingResume(a.session.ID, commandID)
}

func (a *app) restoreRejectedInteractionReview(review *interactionReview, command agent.ResumeRun, failure error) error {
	callFailure, refused := errors.AsType[*resumeRunCallError](failure)
	if refused && a.workbench != nil && errors.Is(callFailure.err, mutation.ErrReplayGuaranteeUnavailable) {
		a.reconcileExpiredResume(command)
		return nil
	}
	if !refused || mutation.OutcomeUnknown(callFailure.err) || a.interactionReview != review ||
		a.conversation.Phase() != agent.ConversationWaiting || a.conversation.RunID() != command.RunID {
		return failure
	}
	if a.workbench != nil {
		if err := a.workbench.RejectPendingResume(a.session.ID, command.CommandID); err != nil {
			return errors.Join(failure, fmt.Errorf("release refused interaction decisions: %w", err))
		}
	}
	a.following = false
	if a.sessionInvalidated {
		a.status.note("resume refused · refreshing session")
		a.refreshInvalidatedSession(false)
		return nil
	}
	a.status.note("resume refused · review preserved")
	review.ReportSubmissionFailure(fmt.Errorf("resume refused: %w", callFailure.err))
	if err := a.reopenCompletedInteractionReview(review); err != nil {
		return errors.Join(failure, err)
	}
	return nil
}

func (a *app) reconcileExpiredResume(command agent.ResumeRun) {
	a.following = false
	a.status.note("resume replay expired · checking runtime state")
	a.syncAnimation()
	sessionID := a.session.ID
	started := a.runSessionSettlement(resumeRecoveryOperation, false,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			return a.readInvalidatedSession(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot, err error) {
			if err != nil {
				a.message("could not reconcile expired interaction delivery: " + err.Error())
				a.status.note("resume outcome unknown · decisions preserved")
				return
			}
			pending, ok := a.workbench.PendingResume(sessionID)
			if !ok || pending.Command.CommandID != command.CommandID {
				return
			}
			if err := a.installSnapshot(snapshot); err != nil {
				a.fail(fmt.Errorf("reconcile expired interaction delivery: %w", err))
				return
			}
			a.restorePendingResume()
		},
	)
	if !started {
		a.status.note("resume outcome unknown · decisions preserved")
	}
}

func (a *app) abortInteractions(reason string) {
	a.clearApprovalProjection()
	a.questionnaire = nil
	a.interactionReview = nil
	if a.reviewDialog != nil {
		a.reviewDialog.Dismiss()
		a.reviewDialog = nil
	}
	if runID := a.conversation.RunID(); runID != "" {
		a.cancelRuntime(agent.CancelRun{RunID: runID, Reason: reason})
	}
}

func (a *app) clearApprovalProjection() {
	a.approval = nil
	a.approvalDraft = nil
	a.approvalArguments = ""
	a.approvalOverride = nil
	a.approvalSections = nil
	a.dismissApprovalEditor()
	a.approvalForm = nil
	a.approvalPane.form = nil
}

func nonEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
