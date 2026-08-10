package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"

	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

func (a *app) registerCommands() {
	for _, found := range a.commands.Find("") {
		a.commands.Remove(found.Command.Name)
	}
	for _, local := range builtinCommands() {
		command := local
		if err := validateLocalCommand(command); err != nil {
			a.message(err.Error())
			continue
		}
		if err := a.addCommand("terminal", headless.Command{
			Name: command.Name, Title: command.Title, Aliases: command.Aliases, Takes: command.Takes,
			Run: func(argument string) {
				if err := runLocalCommandSafely(command, a, argument); err != nil {
					a.message(err.Error())
				}
			},
		}); err != nil {
			a.message(err.Error())
		}
	}
	for _, contributed := range extensions.OwnedValues(a.registry, SlashCommands) {
		command := contributed.Value
		pluginID := contributed.PluginID
		if err := validateCommand(command); err != nil {
			a.message("plugin " + pluginID + ": " + err.Error())
			continue
		}
		if err := a.addCommand(pluginID, headless.Command{
			Name: command.Name, Title: command.Title, Aliases: command.Aliases, Takes: command.Takes,
			Run: func(argument string) { a.executeCommand(pluginID, command, argument) },
		}); err != nil {
			a.message(err.Error())
		}
	}
}

func (a *app) addCommand(owner string, command headless.Command) error {
	identities := append([]string{command.Name}, command.Aliases...)
	for _, identity := range identities {
		if existing, found := a.commands.Lookup(identity); found {
			return fmt.Errorf("plugin %s command /%s conflicts with /%s", owner, command.Name, existing.Name)
		}
	}
	a.commands.Add(command)
	return nil
}

type commandOperation struct {
	pluginID string
	slot     operationSlot
}

func (a *app) executeCommand(pluginID string, command SlashCommand, argument string) {
	a.status.note("running /" + command.Name)
	request := CommandRequest{Argument: argument, Workspace: a.session.Workspace, SessionID: a.session.ID}
	dispatcher := a.loop.Dispatcher()
	a.commandSeq++
	sequence := a.commandSeq
	slot := operationSlot(fmt.Sprintf("command:%d", sequence))
	a.commandOperations[sequence] = commandOperation{pluginID: pluginID, slot: slot}
	started := a.operations.Go(slot, false, func(ctx context.Context, lease operationLease) {
		result, err := executeCommandSafely(ctx, command, request)
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed {
				return
			}
			delete(a.commandOperations, sequence)
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				a.message(err.Error())
				return
			}
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "completed /" + command.Name
			}
			a.message(message)
		})
	})
	if !started {
		delete(a.commandOperations, sequence)
		a.message("could not start /" + command.Name)
	}
}

func (a *app) cancelPluginCommands(pluginIDs ...string) {
	selected := make(map[string]struct{}, len(pluginIDs))
	for _, pluginID := range pluginIDs {
		selected[pluginID] = struct{}{}
	}
	for sequence, operation := range a.commandOperations {
		if len(selected) > 0 {
			if _, cancel := selected[operation.pluginID]; !cancel {
				continue
			}
		}
		a.operations.Cancel(operation.slot)
		delete(a.commandOperations, sequence)
	}
}

func executeCommandSafely(ctx context.Context, command SlashCommand, request CommandRequest) (result CommandResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Name, recovered)
		}
	}()
	return command.Execute(ctx, request)
}

func runLocalCommandSafely(command localCommand, host *app, argument string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("command /%s panicked: %v", command.Name, recovered)
		}
	}()
	return command.Run(host, argument)
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
	a.operations.Go(searchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := a.postSearchResult(ctx, dispatcher, lease, result); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

func (a *app) postSearchResult(ctx context.Context, dispatcher program.Dispatcher, lease operationLease, result headless.Result) error {
	return post(ctx, dispatcher, func() {
		if !a.operations.Current(lease) || a.closed {
			return
		}
		a.acceptSearchResult(result)
	})
}

func (a *app) acceptSearchResult(result headless.Result) {
	if result.Err != nil {
		a.message(fmt.Sprintf("search failed: %v", result.Err))
		return
	}
	accepted, announce := a.transcript.AcceptSearch(result)
	if accepted && announce {
		a.message(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
	}
}

// Local command actions.

func (a *app) Clear() {
	if a.state.Busy() || a.following {
		a.status.doing = "the active run owns the transcript"
		return
	}
	a.state.ClearPresentation()
	a.transcript.Reset()
	a.workflow.Reset()
	a.status = statusView{theme: a.status.theme, glyphs: a.status.glyphs, doing: "cleared", options: a.options}
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
	commands := a.commands.Find("")
	lines := make([]string, 0, len(commands))
	for _, found := range commands {
		command := found.Command
		argument := ""
		if command.Takes {
			argument = " <value>"
		}
		lines = append(lines, fmt.Sprintf("/%-10s %s", command.Name+argument, command.Title))
	}
	a.transcript.Append(&kit.Message{
		Theme: a.transcript.theme, Speaker: "commands", Body: strings.Join(lines, "\n"),
	})
}

func (a *app) AttachFile(path string) error { return a.addAttachment(path) }

func (a *app) DetachFile(value string) error { return a.removeAttachment(value) }

func (a *app) ShowAttachments() { a.showAttachments() }

func (a *app) ToggleToolDetails() {
	a.transcript.ToggleDetails()
	a.message(a.transcript.DetailsLabel())
}
