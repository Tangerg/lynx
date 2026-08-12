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
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type approvalPane struct {
	theme   kit.Theme
	glyphs  kit.Glyphs
	title   string
	detail  *kit.Paragraph
	preview headless.Transcript
	scroll  headless.Scroll
	view    kit.Transcript
	form    *kit.Form
}

func (p *approvalPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	if width <= 0 || height <= 0 || p.form == nil {
		return
	}
	formRows := min(p.form.Measure(width), max(height-1, 0))
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
	p.form.Draw(rows[3])
}

func (p *approvalPane) Handle(event input.Event) bool {
	if p.form != nil && p.form.Handle(event) {
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
	a.approvalDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Tool approval", Body: &a.approvalPane,
		Where: layout.Placement{Width: 88, Height: 24},
	})
}

func (a *app) setApprovalForm(initial approvalAction) {
	rememberable := a.approval == nil || a.approval.Rememberable
	a.approvalChoice = initial.Normalize(rememberable)
	choice := &headless.Select[approvalAction]{
		Label: "How should lyra proceed?", Value: headless.Bind(&a.approvalChoice), Rows: 3,
	}
	choice.SetOptions(approvalOptions(rememberable))
	reason := &headless.Text{
		Label: "Denial feedback (optional)", Placeholder: "Explain what should change before retrying",
		Value: headless.Bind(&a.approvalReason),
	}
	reason.Editor().Clipboard = a.loop.Clipboard()
	keys := headless.DefaultFormKeys()
	a.approvalForm = headless.NewForm(choice, reason)
	a.approvalForm.Keys = keys
	a.approvalForm.Done = func() { a.answerApproval(a.approvalChoice) }
	a.approvalForm.GaveUp = a.backOrCancelApproval
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.approvalPane.theme, Glyphs: a.approvalPane.glyphs, Controller: a.approvalForm,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.approvalPane.form = dressed
}

func (a *app) openApproval(approval agent.Approval) {
	cloned := approval.Clone()
	a.approval = &cloned
	a.approvalReason = ""
	a.approvalArguments = editableApprovalArguments(approval.Tool)
	a.approvalOverride = nil
	initial := defaultApprovalAction(a.settings.Approval.Remember.Scope())
	if answer, ok := a.interactionReview.CurrentAnswer().(agent.ApprovalAnswer); ok {
		initial = approvalActionFromAnswer(answer)
		a.approvalReason = answer.Reason
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
	presentation, err := selectToolPresentation(extensions.Values(a.registry, ToolPresenters), call)
	if err != nil {
		presentation = ToolPresentation{
			Label:    toolLabel(call),
			Sections: []ToolSection{{Title: "Presentation error", Style: toolSectionParagraph, Text: err.Error()}},
		}
	}
	a.approvalSections = slices.Clone(presentation.Sections)
	details := []string{approval.Detail, presentation.Label}
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
	if approval == nil {
		return
	}
	if action == approvalEditArgs {
		a.openApprovalArgumentEditor()
		return
	}
	decision, ok := action.Answer()
	if !ok {
		a.fail(fmt.Errorf("approval action %q is unsupported", action))
		return
	}
	if decision.Decision == agent.ApprovalDeny {
		decision.Reason = strings.TrimSpace(a.approvalReason)
		if decision.Reason == "" {
			decision.Reason = "denied by the user in the terminal"
		}
	} else {
		decision.ArgumentOverride = a.approvalOverride.Clone()
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
	a.approval = nil
	a.approvalOverride = nil
	a.approvalSections = nil
	a.dismissApprovalEditor()
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
		a.approval = nil
		a.approvalOverride = nil
		a.approvalSections = nil
		a.dismissApprovalEditor()
		a.approvalDialog.Dismiss()
	}
	if a.questionnaire != nil {
		a.questionnaire = nil
		a.questionDialog.Dismiss()
	}
	if a.reviewDialog != nil {
		a.reviewDialog.Dismiss()
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
	if a.workbench != nil {
		pending := workbench.PendingResume{Command: command.Clone(), Interactions: review.Items()}
		if err := a.workbench.StagePendingResume(a.session.ID, pending); err != nil {
			a.message("resume blocked: save interaction decisions: " + err.Error())
			return
		}
	}
	a.deliverInteractionResume(review, command)
}

func (a *app) deliverInteractionResume(review *interactionReview, command agent.ResumeRun) {
	a.status.active("resuming")
	a.syncAnimation()
	a.followOpening(func(ctx context.Context) (agent.SegmentStream, error) {
		stream, err := a.runtime.ResumeRun(ctx, command)
		if err != nil {
			return agent.SegmentStream{}, &resumeRunCallError{err: err}
		}
		if err := stream.ValidateResume(nil); err != nil {
			return agent.SegmentStream{}, fmt.Errorf("resume run: %w", err)
		}
		return stream, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(agent.SegmentStream) {
			a.interactionReview = nil
			if a.workbench != nil {
				if err := a.workbench.AcknowledgePendingResume(a.session.ID, command.CommandID); err != nil {
					a.message("could not retire acknowledged interaction decisions: " + err.Error())
				}
			}
		},
		rejected: func(failure error) error {
			return a.restoreRejectedInteractionReview(review, command, failure)
		},
	})
}

func (a *app) restoreRejectedInteractionReview(review *interactionReview, command agent.ResumeRun, failure error) error {
	callFailure, refused := errors.AsType[*resumeRunCallError](failure)
	if !refused || mutation.AcknowledgementUncertain(callFailure.err) || a.interactionReview != review ||
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
	if review.Reviewing() {
		a.openInteractionSummary()
		return nil
	}
	if !review.Back() {
		return errors.Join(failure, errors.New("interaction review cannot return to its submitted answer"))
	}
	a.openCurrentInteraction()
	return nil
}

func (a *app) abortInteractions(reason string) {
	a.approval = nil
	a.approvalOverride = nil
	a.approvalSections = nil
	a.dismissApprovalEditor()
	a.questionnaire = nil
	a.interactionReview = nil
	if a.reviewDialog != nil {
		a.reviewDialog.Dismiss()
	}
	if runID := a.conversation.RunID(); runID != "" {
		a.cancelRuntime(agent.CancelRun{RunID: runID, Reason: reason})
	}
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
