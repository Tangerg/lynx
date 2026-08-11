package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

// SlashCommand is a contributed composer command. Extensions receive a
// bounded request snapshot rather than the terminal application itself.
type SlashCommand struct {
	Name    string
	Title   string
	Aliases []string
	Takes   bool
	Execute func(context.Context, CommandRequest) (CommandResult, error)
}

type localCommand struct {
	Name    string
	Title   string
	Aliases []string
	Takes   bool
	Run     func(*app, string) error
}

// CommandRequest is the bounded product context given to an out-of-process
// slash command.
type CommandRequest struct {
	Argument  string
	Workspace string
	SessionID string
}

// CommandResult is what an asynchronous slash command may surface in the
// terminal chrome.
type CommandResult struct {
	Message string
}

// BlockPresentation is the terminal vocabulary a block presenter receives.
type BlockPresentation struct {
	Theme  kit.Theme
	Glyphs kit.Glyphs
	Look   markdown.Look
	Syntax highlight.Renderer
}

// BlockPresenter maps one closed domain block kind to terminal blocks.
type BlockPresenter struct {
	Kind    agent.BlockKind
	Present func(BlockPresentation, agent.Block) []headless.Block
}

var (
	SlashCommands   = extensions.NewCapabilityKeyedPoint("terminal.slash-command", extensions.Capability("terminal.commands"), func(command SlashCommand) string { return command.Name })
	BlockPresenters = extensions.NewCapabilityKeyedPoint("terminal.block-presenter", extensions.Capability("terminal.presentation"), func(presenter BlockPresenter) string { return string(presenter.Kind) })
)

func builtinPlugin() extensions.Plugin {
	return extensions.Plugin{ID: "terminal.core", Version: "1.0.0", APIVersion: extensions.HostAPIVersion, Trusted: true, Setup: func(scope *extensions.Scope) error {
		presenters := []BlockPresenter{
			{Kind: agent.BlockUser, Present: presentUser},
			{Kind: agent.BlockAssistant, Present: presentMarkdown("lyra")},
			{Kind: agent.BlockReasoning, Present: presentMarkdown("thinking")},
			{Kind: agent.BlockQuestion, Present: presentQuestion},
			{Kind: agent.BlockTool, Present: presentTool},
			{Kind: agent.BlockNotice, Present: presentNotice},
			{Kind: agent.BlockError, Present: presentFailure},
		}
		for i, presenter := range presenters {
			if _, err := extensions.Contribute(scope, BlockPresenters, presenter, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}
		return nil
	}}
}

func builtinCommands() []localCommand {
	return []localCommand{
		{Name: "help", Title: "show commands available in this session", Run: func(a *app, _ string) error { a.ShowHelp(); return nil }},
		{Name: "shortcuts", Title: "show all keyboard shortcuts", Run: func(a *app, _ string) error { a.ShowShortcuts(); return nil }},
		{Name: "clear", Title: "release the live transcript", Run: func(a *app, _ string) error { a.Clear(); return nil }},
		{Name: "find", Title: "find text in the live transcript", Takes: true, Run: func(a *app, query string) error { a.Find(query); return nil }},
		{Name: "next", Title: "step to the next search match", Run: func(a *app, _ string) error { a.NextMatch(); return nil }},
		{Name: "previous", Title: "step to the previous search match", Aliases: []string{"prev"}, Run: func(a *app, _ string) error { a.PreviousMatch(); return nil }},
		{Name: "quit", Title: "leave lyra", Aliases: []string{"exit"}, Run: func(a *app, _ string) error { a.Quit(); return nil }},
		{Name: "sessions", Title: "search and switch sessions", Aliases: []string{"resume"}, Run: func(a *app, _ string) error { a.ShowSessions(); return nil }},
		{Name: "new", Title: "start a new session", Run: func(a *app, _ string) error { a.NewSession(); return nil }},
		{Name: "rename", Title: "rename the current session", Takes: true, Run: func(a *app, title string) error { a.RenameSession(title); return nil }},
		{Name: "fork", Title: "fork the complete current session", Takes: true, Run: func(a *app, title string) error { a.ForkSession(title); return nil }},
		{Name: "model", Title: "choose the model for new runs", Run: func(a *app, _ string) error { a.ChooseModel(); return nil }},
		{Name: "approval", Title: "choose the runtime approval mode", Aliases: []string{"permissions", "permission"}, Run: func(a *app, _ string) error { a.ChooseApprovalMode(); return nil }},
		{Name: "status", Title: "show model, run limits, and runtime approval mode", Run: func(a *app, _ string) error { a.ShowRuntimeStatus(); return nil }},
		{Name: "queue", Title: "manage follow-ups waiting behind the current run", Run: func(a *app, _ string) error { a.ShowQueue(); return nil }},
		{Name: "rules", Title: "show remembered approval rules", Run: func(a *app, _ string) error { a.ShowApprovalRules(); return nil }},
		{Name: "attach", Title: "attach a local file to the next prompt", Takes: true, Run: func(a *app, path string) error { return a.AttachFile(path) }},
		{Name: "detach", Title: "remove an attachment by name, number, or all", Takes: true, Run: func(a *app, value string) error { return a.DetachFile(value) }},
		{Name: "attachments", Title: "show files attached to the next prompt", Aliases: []string{"files"}, Run: func(a *app, _ string) error { a.ShowAttachments(); return nil }},
		{Name: "details", Title: "expand or collapse tool output and diffs", Run: func(a *app, _ string) error { a.ToggleToolDetails(); return nil }},
		{Name: "plugins", Title: "show discovered plugins and lifecycle state", Run: func(a *app, _ string) error { a.ShowPlugins(); return nil }},
		{Name: "reload", Title: "reload a plugin and its dependents", Takes: true, Run: func(a *app, id string) error { a.ReloadPlugin(id); return nil }},
		{Name: "unload", Title: "unload a sideloaded plugin and its dependents", Takes: true, Run: func(a *app, id string) error { a.UnloadPlugin(id); return nil }},
	}
}

func presentUser(p BlockPresentation, block agent.Block) []headless.Block {
	body := strings.TrimSpace(block.Text)
	if len(block.Attachments) > 0 {
		lines := make([]string, 0, len(block.Attachments))
		for _, item := range block.Attachments {
			lines = append(lines, "@"+item.Name+" · "+item.MimeType)
		}
		if body != "" {
			body += "\n\n"
		}
		body += strings.Join(lines, "\n")
	}
	return []headless.Block{newUserMessageBlock(p.Theme, body)}
}

func presentMarkdown(speaker string) func(BlockPresentation, agent.Block) []headless.Block {
	return func(p BlockPresentation, block agent.Block) []headless.Block {
		message := &markdownBlock{theme: p.Theme, speaker: speaker}
		look := p.Look
		if block.Kind == agent.BlockReasoning {
			look.Text, look.Strong = p.Theme.Muted, p.Theme.Subtle
		}
		message.doc.SetBlocks(markdown.Render(block.Text, look))
		return []headless.Block{message}
	}
}

func presentTool(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{newToolBlock(p, block)}
}

func presentQuestion(p BlockPresentation, block agent.Block) []headless.Block {
	if block.Question == nil {
		return nil
	}
	lines := make([]string, 0, len(block.Question.Fields))
	for _, field := range block.Question.Fields {
		lines = append(lines, p.Glyphs.Bullet+" "+field.Prompt)
	}
	body := strings.Join(lines, "\n")
	if block.Question.Detail != "" {
		body = block.Question.Detail + "\n" + body
	}
	return []headless.Block{&kit.Message{Theme: p.Theme, Speaker: block.Question.Title, Body: body}}
}

func presentNotice(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{&kit.Message{Theme: p.Theme, Speaker: "notice", Body: block.Text}}
}

func presentFailure(p BlockPresentation, block agent.Block) []headless.Block {
	return []headless.Block{presentError(p.Theme, block.Text)}
}

func validateCommand(command SlashCommand) error {
	if err := validateCommandMetadata(command.Name, command.Title, command.Aliases); err != nil {
		return err
	}
	if command.Execute == nil {
		return fmt.Errorf("slash command %q has no handler", command.Name)
	}
	return nil
}

func validateLocalCommand(command localCommand) error {
	if err := validateCommandMetadata(command.Name, command.Title, command.Aliases); err != nil {
		return err
	}
	if command.Run == nil {
		return fmt.Errorf("slash command %q has no handler", command.Name)
	}
	return nil
}

func validateCommandMetadata(name, title string, aliases []string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("slash command has no name")
	case strings.ContainsAny(name, " /\t\n"):
		return fmt.Errorf("slash command %q has an invalid name", name)
	case strings.TrimSpace(title) == "":
		return fmt.Errorf("slash command %q has no title", name)
	default:
		return validateCommandAliases(name, aliases)
	}
}

func validateCommandAliases(name string, aliases []string) error {
	seen := map[string]struct{}{name: {}}
	for _, alias := range aliases {
		if strings.TrimSpace(alias) == "" || strings.ContainsAny(alias, " /\t\n") {
			return fmt.Errorf("slash command %q has invalid alias %q", name, alias)
		}
		if _, duplicate := seen[alias]; duplicate {
			return fmt.Errorf("slash command %q repeats name or alias %q", name, alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}
