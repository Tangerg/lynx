package terminal

import (
	"context"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
)

func (a *app) refreshCompletion() {
	a.operations.Cancel(completionOperation)
	lines := strings.Split(a.composer.Text(), "\n")
	line, column := a.composer.Editor().Cursor()
	if line < 0 || line >= len(lines) {
		a.completion.Dismiss()
		return
	}
	token, ok := headless.TokenAt(lines[line], column,
		headless.Trigger{Prefix: "/", AtStart: true},
		headless.Trigger{Prefix: "@"},
	)
	if !ok {
		a.completion.Dismiss()
		return
	}
	if token.Trigger.Prefix == "@" {
		a.completeFiles(token)
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

func (a *app) completeFiles(token headless.Token) {
	if a.attachments == nil {
		a.completion.Dismiss()
		return
	}
	resolver := a.attachments
	runOperation(a, completionOperation, true,
		func(ctx context.Context) ([]headless.Candidate, error) {
			matches, err := resolver.Complete(ctx, token.Query, 50)
			candidates := make([]headless.Candidate, 0, len(matches))
			for _, match := range matches {
				candidates = append(candidates, headless.Candidate{
					Text: match.Path, Label: match.Path, Detail: match.Detail, Matched: match.Matched,
				})
			}
			return candidates, err
		},
		func(candidates []headless.Candidate, err error) {
			if err != nil {
				a.completion.Dismiss()
				a.message(err.Error())
				return
			}
			a.completion.Offer(token, candidates)
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
