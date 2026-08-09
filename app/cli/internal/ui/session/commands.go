package session

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

func (a *app) registerCommands() {
	for _, contributed := range extensions.Values(a.registry, SlashCommands) {
		command := contributed
		if err := validateCommand(command); err != nil {
			a.message(err.Error())
			continue
		}
		a.commands.Add(headless.Command{
			Name: command.Name, Title: command.Title, Aliases: command.Aliases, Takes: command.Takes,
			Run: func(argument string) {
				if err := command.Run(a, argument); err != nil {
					a.message(err.Error())
				}
			},
		})
	}
}

func (a *app) runCommand(name, argument string) {
	command, ok := a.commands.Lookup(name)
	if !ok || command.Run == nil {
		a.message("unknown command: /" + name)
		return
	}
	if command.Takes && strings.TrimSpace(argument) == "" {
		a.message("/" + command.Name + " needs an argument")
		return
	}
	a.commands.Used(command.Name)
	command.Run(strings.TrimSpace(argument))
}

func (a *app) refreshCompletion() {
	lines := strings.Split(a.composer.Text(), "\n")
	line, column := a.composer.Editor().Cursor()
	if line < 0 || line >= len(lines) {
		a.completion.Dismiss()
		return
	}
	token, ok := headless.TokenAt(lines[line], column, headless.Trigger{Prefix: "/", AtStart: true})
	if !ok {
		a.completion.Dismiss()
		return
	}
	found := a.commands.Find(token.Query)
	candidates := make([]headless.Candidate, 0, len(found))
	for _, match := range found {
		candidates = append(candidates, headless.Candidate{
			Text: match.Command.Name, Label: match.Command.Name,
			Detail: match.Command.Title, Matched: match.At,
		})
	}
	a.completion.Offer(token, candidates)
}

func (a *app) drawCompletion(frame headless.Frame) {
	width, height := frame.Size()
	rows := a.completion.Measure(width)
	if width <= 2 || height <= 2 || rows <= 0 {
		return
	}
	box := kit.Box{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Padding: layout.Symmetric(0, 1), Title: "commands", Footer: "enter complete",
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

func (a *app) listenForSearch() {
	results := a.transcript.SearchResults()
	dispatcher := a.loop.Dispatcher()
	go func() {
		for result := range results {
			result := result
			dispatcher.Post(func() {
				if result.Err != nil {
					a.message(fmt.Sprintf("search failed: %v", result.Err))
					return
				}
				if !a.transcript.AcceptSearch(result) {
					return
				}
				a.message(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
			})
		}
	}()
}

// CommandHost implementation.

func (a *app) Clear() {
	if a.state.Busy() || a.following {
		a.status.doing = "the active run owns the transcript"
		return
	}
	a.state.Reset()
	a.transcript.Reset()
	a.workflow.Reset()
	a.status = statusView{theme: a.status.theme, glyphs: a.status.glyphs, doing: "cleared"}
}

func (a *app) Find(query string) {
	a.transcript.Find(query)
	a.message("searching for " + query)
}

func (a *app) NextMatch() {
	if !a.transcript.StepMatch(1) {
		a.message("no active search matches")
	}
}

func (a *app) PreviousMatch() {
	if !a.transcript.StepMatch(-1) {
		a.message("no active search matches")
	}
}

func (a *app) Quit() { a.loop.Quit() }

func (a *app) ShowHelp() {
	var lines []string
	for _, command := range extensions.Values(a.registry, SlashCommands) {
		argument := ""
		if command.Takes {
			argument = " <value>"
		}
		lines = append(lines, fmt.Sprintf("/%-10s %s", command.Name+argument, command.Title))
	}
	a.transcript.Append(kit.Message{
		Theme: a.transcript.theme, Speaker: "commands", Body: strings.Join(lines, "\n"),
	})
}

func (a *app) SetStatus(message string) { a.message(message) }

func (a *app) message(label string) {
	if a.state.Phase() == client.Running {
		a.status.active(label)
		return
	}
	a.status.note(label)
}
