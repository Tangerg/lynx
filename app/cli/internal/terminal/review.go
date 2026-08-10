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

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type reviewPane struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	title    string
	detail   *kit.Paragraph
	code     *kit.Code
	viewport *headless.Viewport
	form     *kit.Form
}

func (p *reviewPane) Draw(frame headless.Frame) {
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

func (p *reviewPane) Handle(event input.Event) bool {
	if p.form != nil && p.form.Handle(event) {
		return true
	}
	return p.viewport.Handle(event)
}

func (p *reviewPane) Focus(has bool) {
	if p.form != nil {
		p.form.Focus(has)
	}
}

func (a *app) buildReview(theme kit.Theme, glyphs kit.Glyphs) {
	code := kit.NewCode(nil)
	code.Gutter = kit.LineNumbers{Style: theme.Subtle, Separator: glyphs.Vertical}
	a.reviewPane = reviewPane{
		theme: theme, glyphs: glyphs, detail: kit.NewParagraph("", theme.Text), code: code,
		viewport: headless.NewViewport(headless.Static{Of: code}),
	}
	a.setReviewForm("allow-once")
	a.reviewDialog = kit.NewDialog(&a.stack, theme, glyphs, "Tool approval", &a.reviewPane)
	a.reviewDialog.Panel().Where = layout.Placement{Width: 88, Height: 24, Margin: 1}
}

func (a *app) setReviewForm(initial string) {
	a.reviewChoice = initial
	choice := &headless.Select[string]{
		Label: "How should lyra proceed?", Value: headless.Bind(&a.reviewChoice), Rows: 5,
	}
	choice.SetOptions([]headless.Option[string]{
		{Label: "Allow once", Value: "allow-once"},
		{Label: "Allow for this session", Value: "allow-session"},
		{Label: "Allow for this project", Value: "allow-project"},
		{Label: "Always allow this rule", Value: "allow-global"},
		{Label: "Deny", Value: "deny"},
	})
	keys := headless.DefaultFormKeys()
	a.reviewForm = headless.NewForm(choice)
	a.reviewForm.Keys = keys
	a.reviewForm.Done = func() { a.answerReview(a.reviewChoice) }
	a.reviewForm.GaveUp = func() { a.answerReview("deny") }
	dressed := kit.NewForm(a.reviewPane.theme, a.reviewPane.glyphs, a.reviewForm)
	dressed.Keys = keys
	dressed.Hints = []keymap.Action{headless.Submit, headless.Cancel}
	a.reviewPane.form = dressed
}

func (a *app) openReview(approval client.Approval) {
	cloned := approval
	a.review = &cloned
	a.setReviewForm(approvalDefault(a.settings.Approval.Remember))
	a.reviewPane.title = approval.Title
	details := []string{approval.Detail}
	if approval.Risk != "" {
		details = append(details, "risk: "+approval.Risk)
	}
	if approval.RuleHint != "" {
		details = append(details, "rule: "+approval.RuleHint)
	}
	a.reviewPane.detail.SetText([]text.Line{text.Of(strings.Join(nonEmpty(details), "\n"), a.reviewPane.theme.Text)})
	diff := strings.TrimSpace(approval.Diff)
	if diff == "" {
		diff = "No diff was supplied for this request."
	}
	a.reviewPane.code.SetText(highlight.Lines("diff", diff, a.syntax))
	a.reviewPane.viewport.Scroll().ToTop()
	a.reviewDialog.Controller().SetDescription(approval.Title)
	a.reviewDialog.Show()
}

func (a *app) openInteraction(interaction client.Interaction) {
	if a.review != nil || a.question != nil {
		a.fail(errors.New("runtime opened a second interaction while one is active"))
		return
	}
	switch item := interaction.(type) {
	case client.Approval:
		a.openReview(item)
	case client.Question:
		a.openQuestion(item)
	default:
		a.fail(errors.New("runtime returned an unknown interaction"))
	}
}

func (a *app) answerReview(choice string) {
	approval := a.review
	if approval == nil {
		return
	}
	a.review = nil
	a.reviewDialog.Dismiss()
	a.status.active("resuming")
	a.syncAnimation()
	decision := approvalAnswer(choice)
	if decision.Decision == client.ApprovalDeny {
		decision.Reason = "denied by the user in the terminal"
	}
	a.resumeInteraction(approval.InterruptID, decision)
}

func approvalDefault(scope client.RememberScope) string {
	switch scope {
	case client.RememberSession:
		return "allow-session"
	case client.RememberProject:
		return "allow-project"
	case client.RememberGlobal:
		return "allow-global"
	case client.RememberNone:
		return "allow-once"
	default:
		return "allow-once"
	}
}

func approvalAnswer(choice string) client.ApprovalAnswer {
	switch choice {
	case "allow-session":
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberSession}
	case "allow-project":
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberProject}
	case "allow-global":
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberGlobal}
	case "allow-once":
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberNone}
	default:
		return client.ApprovalAnswer{Decision: client.ApprovalDeny, Remember: client.RememberNone}
	}
}

func (a *app) resumeInteraction(interruptID string, answer client.Answer) {
	runID := a.state.RunID()
	after := a.state.Cursor()
	a.follow(func(ctx context.Context) (subscription, error) {
		if err := a.runtime.ResumeRun(ctx, client.ResumeRun{RunID: runID, InterruptID: interruptID, Answer: answer}); err != nil {
			return subscription{}, err
		}
		return subscription{runID: runID, after: after}, nil
	})
}

func nonEmpty(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
