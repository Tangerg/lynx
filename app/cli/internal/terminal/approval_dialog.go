package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type approvalPane struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	title    string
	detail   *kit.Paragraph
	code     *kit.Code
	viewport *headless.Viewport
	form     *kit.Form
}

func (p *approvalPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	if width <= 0 || height <= 0 || p.form == nil {
		return
	}
	detailRows := min(p.detail.Measure(width), 4)
	formRows := min(p.form.Measure(width), 7)
	rows := frame.Subs(layout.Down.Rects(frame.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(detailRows)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(formRows)},
	))
	kit.Label{Text: p.title, Style: p.theme.Strong, Ellipsis: "…"}.Draw(rows[0].View)
	p.detail.Draw(rows[1].View)
	p.viewport.Draw(rows[2])
	p.form.Draw(rows[3])
}

func (p *approvalPane) Handle(event input.Event) bool {
	if p.form != nil && p.form.Handle(event) {
		return true
	}
	return p.viewport.Handle(event)
}

func (p *approvalPane) Focus(has bool) {
	if p.form != nil {
		p.form.Focus(has)
	}
}

func (a *app) buildApprovalDialog(theme kit.Theme, glyphs kit.Glyphs) {
	code := kit.NewCode(nil)
	code.Gutter = kit.LineNumbers{Style: theme.Subtle, Separator: glyphs.Vertical}
	a.approvalPane = approvalPane{
		theme: theme, glyphs: glyphs, detail: kit.NewParagraph("", theme.Text), code: code,
		viewport: headless.NewViewport(headless.Static{Of: code}),
	}
	a.setApprovalForm("allow-once")
	a.approvalDialog = kit.NewDialog(&a.stack, theme, glyphs, "Tool approval", &a.approvalPane)
	a.approvalDialog.Panel().Where = layout.Placement{Width: 88, Height: 24, Margin: 1}
}

func (a *app) setApprovalForm(initial string) {
	a.approvalChoice = initial
	choice := &headless.Select[string]{
		Label: "How should lyra proceed?", Value: headless.Bind(&a.approvalChoice), Rows: 5,
	}
	choice.SetOptions([]headless.Option[string]{
		{Label: "Allow once", Value: "allow-once"},
		{Label: "Allow for this session", Value: "allow-session"},
		{Label: "Allow for this project", Value: "allow-project"},
		{Label: "Always allow this rule", Value: "allow-global"},
		{Label: "Deny", Value: "deny"},
	})
	keys := headless.DefaultFormKeys()
	a.approvalForm = headless.NewForm(choice)
	a.approvalForm.Keys = keys
	a.approvalForm.Done = func() { a.answerApproval(a.approvalChoice) }
	a.approvalForm.GaveUp = func() { a.answerApproval("deny") }
	dressed := kit.NewForm(a.approvalPane.theme, a.approvalPane.glyphs, a.approvalForm)
	dressed.Keys = keys
	dressed.Hints = []keymap.Action{headless.Submit, headless.Cancel}
	a.approvalPane.form = dressed
}

func (a *app) openApproval(approval agent.Approval) {
	cloned := approval
	a.approval = &cloned
	a.setApprovalForm(approvalDefault(a.settings.Approval.Remember))
	a.approvalPane.title = approval.Title
	details := []string{approval.Detail}
	if approval.Risk != "" {
		details = append(details, "risk: "+approval.Risk)
	}
	if approval.RuleHint != "" {
		details = append(details, "rule: "+approval.RuleHint)
	}
	a.approvalPane.detail.SetText([]text.Line{text.Of(strings.Join(nonEmptyStrings(details), "\n"), a.approvalPane.theme.Text)})
	diff := strings.TrimSpace(approval.Diff)
	if diff == "" {
		diff = "No diff was supplied for this request."
	}
	a.approvalPane.code.SetText(highlight.Lines("diff", diff, a.syntax))
	a.approvalPane.viewport.Scroll().ToTop()
	a.approvalDialog.Controller().SetDescription(approval.Title)
	a.approvalDialog.Show()
}

func (a *app) openInteraction(interaction agent.Interaction) {
	if a.approval != nil || a.question != nil {
		a.fail(errors.New("runtime opened a second interaction while one is active"))
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

func (a *app) answerApproval(choice string) {
	approval := a.approval
	if approval == nil {
		return
	}
	a.approval = nil
	a.approvalDialog.Dismiss()
	a.status.active("resuming")
	a.syncAnimation()
	decision := approvalAnswer(choice)
	if decision.Decision == agent.ApprovalDeny {
		decision.Reason = "denied by the user in the terminal"
	}
	a.resumeInteraction(approval.InterruptID, decision)
}

func approvalDefault(scope agent.RememberScope) string {
	switch scope {
	case agent.RememberSession:
		return "allow-session"
	case agent.RememberProject:
		return "allow-project"
	case agent.RememberGlobal:
		return "allow-global"
	case agent.RememberNone:
		return "allow-once"
	default:
		return "allow-once"
	}
}

func approvalAnswer(choice string) agent.ApprovalAnswer {
	switch choice {
	case "allow-session":
		return agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberSession}
	case "allow-project":
		return agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberProject}
	case "allow-global":
		return agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberGlobal}
	case "allow-once":
		return agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberNone}
	default:
		return agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberNone}
	}
}

func (a *app) resumeInteraction(interruptID string, answer agent.Answer) {
	runID := a.conversation.RunID()
	after := a.conversation.Cursor()
	a.follow(func(ctx context.Context) (runSubscription, error) {
		if err := a.runtime.ResumeRun(ctx, agent.ResumeRun{RunID: runID, InterruptID: interruptID, Answer: answer}); err != nil {
			return runSubscription{}, err
		}
		return runSubscription{runID: runID, after: after}, nil
	})
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
