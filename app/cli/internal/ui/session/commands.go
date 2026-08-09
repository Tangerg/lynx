package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	a.completionSeq++
	sequence := a.completionSeq
	if a.completionCancel != nil {
		a.completionCancel()
		a.completionCancel = nil
	}
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
		a.completeFiles(sequence, token)
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

func (a *app) completeFiles(sequence uint64, token headless.Token) {
	if a.attachments == nil {
		a.completion.Dismiss()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.completionCancel = cancel
	dispatcher := a.loop.Dispatcher()
	go func() {
		matches, err := a.attachments.Complete(ctx, token.Query, 50)
		if errors.Is(err, context.Canceled) {
			return
		}
		candidates := make([]headless.Candidate, 0, len(matches))
		for _, match := range matches {
			candidates = append(candidates, headless.Candidate{
				Text: match.Path, Label: match.Path, Detail: match.Detail, Matched: match.Matched,
			})
		}
		dispatcher.Post(func() {
			if a.completionSeq != sequence || a.closed {
				return
			}
			a.completionCancel = nil
			if err != nil {
				a.completion.Dismiss()
				a.message(err.Error())
				return
			}
			a.completion.Offer(token, candidates)
		})
	}()
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
	a.state.ClearPresentation()
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

func (a *app) AttachFile(path string) error { return a.addAttachment(path) }

func (a *app) DetachFile(value string) error { return a.removeAttachment(value) }

func (a *app) ShowAttachments() { a.showAttachments() }

func (a *app) ShowSessions() {
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run before switching sessions")
		return
	}
	a.message("loading sessions")
	dispatcher := a.loop.Dispatcher()
	go func() {
		page, err := a.backend.ListSessions(a.ctx, client.SessionQuery{Limit: 100})
		_ = post(a.ctx, dispatcher, func() {
			if err != nil {
				a.fail(err)
				return
			}
			a.sessionPicker.Reset()
			a.sessionPicker.SetItems(page.Items)
			a.sessionDialog.Show()
			a.status.note("choose a session")
		})
	}()
}

func (a *app) NewSession() {
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run before creating a session")
		return
	}
	a.switchSeq++
	sequence := a.switchSeq
	a.message("creating session")
	dispatcher := a.loop.Dispatcher()
	go func() {
		created, err := a.backend.CreateSession(a.ctx, client.NewSession{Workspace: a.session.Workspace})
		_ = post(a.ctx, dispatcher, func() {
			if sequence != a.switchSeq {
				return
			}
			if err != nil {
				a.fail(err)
				return
			}
			a.installSnapshot(client.SessionSnapshot{Session: created})
		})
	}()
}

func (a *app) RenameSession(title string) {
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run before renaming the session")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		a.message("/rename needs a non-empty title")
		return
	}
	session := a.session
	dispatcher := a.loop.Dispatcher()
	go func() {
		latest, err := a.backend.GetSession(a.ctx, session.ID)
		var updated client.Session
		if err == nil {
			updated, err = a.backend.UpdateSession(a.ctx, client.UpdateSession{SessionID: session.ID, Title: title, Revision: latest.Session.Revision})
		}
		_ = post(a.ctx, dispatcher, func() {
			if err != nil {
				a.fail(err)
				return
			}
			a.session = updated
			a.loop.Session().SetTitle("lyra — " + displayTitle(updated))
			a.message("renamed session to " + updated.Title)
		})
	}()
}

func (a *app) ForkSession(title string) {
	if a.state.Busy() || a.following {
		a.message("finish or cancel the current run before forking the session")
		return
	}
	a.switchSeq++
	sequence := a.switchSeq
	source, at := a.session.ID, a.state.Cursor()
	dispatcher := a.loop.Dispatcher()
	go func() {
		forked, err := a.backend.ForkSession(a.ctx, client.ForkSession{SessionID: source, At: at, Title: strings.TrimSpace(title)})
		var snapshot client.SessionSnapshot
		if err == nil {
			snapshot, err = a.backend.GetSession(a.ctx, forked.ID)
		}
		_ = post(a.ctx, dispatcher, func() {
			if sequence != a.switchSeq {
				return
			}
			if err != nil {
				a.fail(err)
				return
			}
			a.installSnapshot(snapshot)
		})
	}()
}

func (a *app) switchSession(id string) {
	if id == a.session.ID {
		a.message("already in " + displayTitle(a.session))
		return
	}
	a.switchSeq++
	sequence := a.switchSeq
	a.message("loading session")
	dispatcher := a.loop.Dispatcher()
	go func() {
		snapshot, err := a.backend.GetSession(a.ctx, id)
		_ = post(a.ctx, dispatcher, func() {
			if sequence != a.switchSeq {
				return
			}
			if err != nil {
				a.fail(err)
				return
			}
			a.installSnapshot(snapshot)
		})
	}()
}

func (a *app) installSnapshot(snapshot client.SessionSnapshot) {
	a.dropStream()
	a.session = snapshot.Session
	a.state = client.NewConversation()
	a.transcript.Reset()
	a.workflow.Reset()
	a.status = statusView{theme: a.status.theme, glyphs: a.status.glyphs, doing: "ready", options: a.options}
	a.loop.Session().SetTitle("lyra — " + displayTitle(snapshot.Session))
	a.restore(snapshot)
	if a.state.Phase() == client.Idle {
		a.message("session · " + displayTitle(snapshot.Session))
	}
}

func agoShort(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	duration := time.Since(at)
	switch {
	case duration < time.Minute:
		return "now"
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

func (a *app) message(label string) {
	if a.state.Phase() == client.Running {
		a.status.active(label)
		return
	}
	a.status.note(label)
}
