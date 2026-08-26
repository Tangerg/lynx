package terminal

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type completionQuery struct {
	line  int
	token headless.Token
}

// completionGate remembers a token whose popup was explicitly closed. A key-up
// event, repaint, rejected candidate, or late async result must not immediately
// reopen it; editing the token or leaving it explicitly starts a new query.
type completionGate struct {
	suppressed completionQuery
	active     bool
}

func (c *completionGate) Allow(query completionQuery) bool {
	if !c.active {
		return true
	}
	if c.suppressed == query {
		return false
	}
	c.Reset()
	return true
}

func (c *completionGate) Suppress(query completionQuery) {
	c.suppressed, c.active = query, true
}

func (c *completionGate) Reset() {
	c.suppressed, c.active = completionQuery{}, false
}

func (a *app) refreshCompletion() {
	a.operations.Cancel(completionOperation)
	query, ok := a.currentCompletionQuery()
	if !ok {
		a.completionGate.Reset()
		a.completion.Dismiss()
		return
	}
	if !a.completionGate.Allow(query) {
		a.completion.Dismiss()
		return
	}
	token := query.token
	if token.Trigger.Prefix == "@" {
		a.completeFiles(query)
		return
	}
	found := a.commands.find(token.Query)
	candidates := make([]headless.Candidate, 0, len(found))
	for _, match := range found {
		availability := a.commands.availability(match.Command.Name, a)
		detail := match.Command.Title
		if !availability.Enabled {
			detail = "unavailable: " + availability.Reason
		}
		candidates = append(candidates, headless.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: detail, Matched: match.At,
		})
	}
	a.completion.Offer(token, candidates)
}

func (a *app) currentCompletionQuery() (completionQuery, bool) {
	lines := strings.Split(a.composer.Text(), "\n")
	line, column := a.composer.Editor().Cursor()
	if line < 0 || line >= len(lines) {
		return completionQuery{}, false
	}
	token, ok := headless.TokenAt(lines[line], column,
		headless.Trigger{Prefix: "/", AtStart: true},
		headless.Trigger{Prefix: "@"},
	)
	if !ok {
		return completionQuery{}, false
	}
	return completionQuery{line: line, token: token}, true
}

func (a *app) handleCompletion(event input.Event) bool {
	_, wasOpen := a.completion.Token()
	handled := a.completion.Handle(event)
	if !handled || !wasOpen || a.completion.Open() {
		return handled
	}
	if query, ok := a.currentCompletionQuery(); ok {
		a.completionGate.Suppress(query)
	} else {
		a.completionGate.Reset()
	}
	return true
}

func (a *app) completeFiles(query completionQuery) {
	if a.attachments == nil {
		a.completion.Dismiss()
		return
	}
	resolver := a.attachments
	a.runOperation(completionOperation, true,
		func(ctx context.Context) ([]headless.Candidate, error) {
			matches, err := resolver.Complete(ctx, query.token.Query, 50)
			candidates := make([]headless.Candidate, 0, len(matches))
			for _, match := range matches {
				candidates = append(candidates, headless.Candidate{
					Text: match.Path, Label: match.Path, Detail: match.Detail, Matched: match.Matched,
				})
			}
			return candidates, err
		},
		func(candidates []headless.Candidate, err error) {
			if !a.completionGate.Allow(query) {
				a.completion.Dismiss()
				return
			}
			if err != nil {
				a.completion.Dismiss()
				a.message(err.Error())
				return
			}
			a.completion.Offer(query.token, candidates)
		},
	)
}

func (a *app) exactCommandCompletion() bool {
	token, open := a.completion.Token()
	if !open || token.Trigger.Prefix != "/" || token.Query == "" {
		return false
	}
	command, found := a.commands.lookup(token.Query)
	if !found {
		return false
	}
	candidate, selected := a.completion.Current()
	return selected && candidate.Text == command.Name
}

func (a *app) drawCompletion(frame headless.Frame) {
	width, height := frame.Size()
	rows := a.completion.Measure(width)
	if width <= 2 || height <= 2 || rows <= 0 {
		return
	}
	title := "commands"
	footer := "enter complete"
	if token, ok := a.completion.Token(); ok && token.Trigger.Prefix == "@" {
		title = "workspace files"
		footer = "enter attach"
	} else if a.exactCommandCompletion() {
		footer = "enter run"
	}
	box := kit.Box{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Padding: layout.Symmetric(0, 1), Title: title, Footer: footer,
		FooterAlign: layout.End,
	}
	popupWidth := min(max(a.completion.Width()+4, 32), width-2)
	popupHeight := min(rows+2, height)
	y := max(height-a.composer.Measure(width)-popupHeight, 0)
	area := grid.Rect(1, y, popupWidth, popupHeight)
	inner := box.InnerRect(area.Size())
	box.Draw(frame.View.Sub(area))
	a.completion.Draw(frame.Sub(area).Sub(inner))
}
