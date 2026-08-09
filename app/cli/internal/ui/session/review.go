package session

import (
	"context"
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
	title    string
	detail   *kit.Paragraph
	code     *kit.Code
	viewport *headless.Viewport
	form     *kit.Form
}

func (p *reviewPane) Draw(frame headless.Frame) {
	width, height := frame.Size()
	if width <= 0 || height <= 0 {
		return
	}
	detailRows := min(p.detail.Measure(width), 3)
	formRows := min(p.form.Measure(width), 3)
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
	if p.form.Handle(event) {
		return true
	}
	return p.viewport.Handle(event)
}

func (p *reviewPane) Focus(has bool) { p.form.Focus(has) }

func (a *app) buildReview(theme kit.Theme, glyphs kit.Glyphs) {
	a.reviewAnswer = true
	a.confirm = &headless.Confirm{
		Label: "Allow this tool call?", Value: headless.Bind(&a.reviewAnswer),
		Yes: "allow once", No: "deny",
	}
	keys := headless.DefaultFormKeys()
	a.reviewForm = headless.NewForm(a.confirm)
	a.reviewForm.Keys = keys
	a.reviewForm.Done = func() { a.answerReview(a.reviewAnswer) }
	a.reviewForm.GaveUp = func() { a.answerReview(false) }
	dressed := kit.NewForm(theme, glyphs, a.reviewForm)
	dressed.Keys = keys
	dressed.Hints = []keymap.Action{headless.Submit, headless.Cancel}
	code := kit.NewCode(nil)
	code.Gutter = kit.LineNumbers{Style: theme.Subtle, Separator: glyphs.Vertical}
	a.reviewPane = reviewPane{
		theme: theme, detail: kit.NewParagraph("", theme.Text), code: code,
		viewport: headless.NewViewport(headless.Static{Of: code}), form: dressed,
	}
	a.reviewDialog = kit.NewDialog(&a.stack, theme, glyphs, "Tool approval", &a.reviewPane)
	a.reviewDialog.Panel().Where = layout.Placement{Width: 82, Height: 20, Margin: 1}
}

func (a *app) openReview(approval client.Approval) {
	if a.review != nil {
		a.answerReview(false)
	}
	copy := approval
	a.review = &copy
	a.reviewAnswer = true
	a.confirm.Say(true)
	a.reviewPane.title = approval.Title
	a.reviewPane.detail.SetText([]text.Line{text.Of(approval.Detail, a.reviewPane.theme.Text)})
	diff := strings.TrimSpace(approval.Diff)
	if diff == "" {
		diff = "No diff was supplied for this request."
	}
	a.reviewPane.code.SetText(highlight.Lines("diff", diff, a.syntax))
	a.reviewPane.viewport.Scroll().ToTop()
	a.reviewDialog.Controller().SetDescription(approval.Title)
	a.reviewDialog.Show()
}

func (a *app) answerReview(approved bool) {
	approval := a.review
	if approval == nil {
		return
	}
	a.review = nil
	a.reviewDialog.Dismiss()
	if !a.state.Resumed() {
		a.fail(client.ErrInterruptNotOpen)
		return
	}
	a.status.active("resuming")
	a.syncAnimation()
	decision := client.Decision{Approved: approved}
	if !approved {
		decision.Reason = "denied by the user in the terminal"
	}
	runID := a.state.RunID()
	a.follow(func(ctx context.Context) (client.Stream, error) {
		return a.backend.ResumeRun(ctx, client.ResumeRun{
			RunID: runID, InterruptID: approval.InterruptID, Decision: decision,
		})
	})
}
