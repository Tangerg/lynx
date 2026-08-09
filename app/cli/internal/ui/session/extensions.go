package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

// CommandHost is the product-level surface available to slash commands.
type CommandHost interface {
	Clear()
	Find(string)
	NextMatch()
	PreviousMatch()
	Quit()
	ShowHelp()
	SetStatus(string)
	ShowSessions()
	NewSession()
	RenameSession(string)
	ForkSession(string)
	ChooseModel()
	CycleMode()
	ChoosePermission()
	SetEffort(string)
	ShowRuntimeStatus()
	ShowApprovalRules()
	AttachFile(string) error
	DetachFile(string) error
	ShowAttachments()
	ToggleToolDetails()
	ShowPlugins()
	ReloadPlugin(string)
	UnloadPlugin(string)
}

// SlashCommand is a contributed composer command.
type SlashCommand struct {
	Name    string
	Title   string
	Aliases []string
	Takes   bool
	Run     func(CommandHost, string) error
	Execute func(context.Context, CommandRequest) (CommandResult, error)
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

// Presentation is the terminal vocabulary a block presenter receives.
type Presentation struct {
	Theme  kit.Theme
	Glyphs kit.Glyphs
	Look   markdown.Look
	Syntax highlight.Style
}

// BlockPresenter maps one closed domain block kind to terminal blocks.
type BlockPresenter struct {
	Kind    client.BlockKind
	Present func(Presentation, client.Block) []headless.Block
}

var (
	SlashCommands   = extensions.NewCapabilityKeyedPoint("terminal.slash-command", extensions.Capability("terminal.commands"), func(command SlashCommand) string { return command.Name })
	BlockPresenters = extensions.NewCapabilityKeyedPoint("terminal.block-presenter", extensions.Capability("terminal.presentation"), func(presenter BlockPresenter) string { return string(presenter.Kind) })
)

func builtinPlugin() extensions.Plugin {
	return extensions.Plugin{ID: "terminal.core", Version: "1.0.0", APIVersion: extensions.HostAPIVersion, Trusted: true, Setup: func(scope *extensions.Scope) error {
		commands := []SlashCommand{
			{Name: "help", Title: "show commands available in this session", Run: func(host CommandHost, _ string) error { host.ShowHelp(); return nil }},
			{Name: "clear", Title: "release the live transcript", Run: func(host CommandHost, _ string) error { host.Clear(); return nil }},
			{Name: "find", Title: "find text in the live transcript", Takes: true, Run: func(host CommandHost, query string) error { host.Find(query); return nil }},
			{Name: "next", Title: "step to the next search match", Run: func(host CommandHost, _ string) error { host.NextMatch(); return nil }},
			{Name: "previous", Title: "step to the previous search match", Aliases: []string{"prev"}, Run: func(host CommandHost, _ string) error { host.PreviousMatch(); return nil }},
			{Name: "quit", Title: "leave lyra", Aliases: []string{"exit"}, Run: func(host CommandHost, _ string) error { host.Quit(); return nil }},
			{Name: "sessions", Title: "search and switch sessions", Aliases: []string{"resume"}, Run: func(host CommandHost, _ string) error { host.ShowSessions(); return nil }},
			{Name: "new", Title: "start a new session", Run: func(host CommandHost, _ string) error { host.NewSession(); return nil }},
			{Name: "rename", Title: "rename the current session", Takes: true, Run: func(host CommandHost, title string) error { host.RenameSession(title); return nil }},
			{Name: "fork", Title: "fork the current session at its latest event", Takes: true, Run: func(host CommandHost, title string) error { host.ForkSession(title); return nil }},
			{Name: "model", Title: "choose the model for new runs", Run: func(host CommandHost, _ string) error { host.ChooseModel(); return nil }},
			{Name: "mode", Title: "cycle build, plan, and review modes", Run: func(host CommandHost, _ string) error { host.CycleMode(); return nil }},
			{Name: "permissions", Title: "choose the permission mode", Aliases: []string{"permission"}, Run: func(host CommandHost, _ string) error { host.ChoosePermission(); return nil }},
			{Name: "effort", Title: "set reasoning effort", Takes: true, Run: func(host CommandHost, value string) error { host.SetEffort(value); return nil }},
			{Name: "status", Title: "show model, mode, permission, and effort", Run: func(host CommandHost, _ string) error { host.ShowRuntimeStatus(); return nil }},
			{Name: "rules", Title: "show remembered approval rules", Run: func(host CommandHost, _ string) error { host.ShowApprovalRules(); return nil }},
			{Name: "attach", Title: "attach a local file to the next prompt", Takes: true, Run: func(host CommandHost, path string) error { return host.AttachFile(path) }},
			{Name: "detach", Title: "remove an attachment by name, number, or all", Takes: true, Run: func(host CommandHost, value string) error { return host.DetachFile(value) }},
			{Name: "attachments", Title: "show files attached to the next prompt", Aliases: []string{"files"}, Run: func(host CommandHost, _ string) error { host.ShowAttachments(); return nil }},
			{Name: "details", Title: "expand or collapse tool output and diffs", Run: func(host CommandHost, _ string) error { host.ToggleToolDetails(); return nil }},
			{Name: "plugins", Title: "show discovered plugins and lifecycle state", Run: func(host CommandHost, _ string) error { host.ShowPlugins(); return nil }},
			{Name: "reload", Title: "reload a plugin and its dependents", Takes: true, Run: func(host CommandHost, id string) error { host.ReloadPlugin(id); return nil }},
			{Name: "unload", Title: "unload a sideloaded plugin and its dependents", Takes: true, Run: func(host CommandHost, id string) error { host.UnloadPlugin(id); return nil }},
		}
		for i, command := range commands {
			if _, err := extensions.Contribute(scope, SlashCommands, command, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}

		presenters := []BlockPresenter{
			{Kind: client.BlockUser, Present: presentUser},
			{Kind: client.BlockAssistant, Present: presentMarkdown("lyra")},
			{Kind: client.BlockReasoning, Present: presentMarkdown("thinking")},
			{Kind: client.BlockTool, Present: presentTool},
			{Kind: client.BlockNotice, Present: presentNotice},
			{Kind: client.BlockError, Present: presentFailure},
		}
		for i, presenter := range presenters {
			if _, err := extensions.Contribute(scope, BlockPresenters, presenter, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}
		return nil
	}}
}

func presentUser(p Presentation, block client.Block) []headless.Block {
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
	return []headless.Block{kit.Message{Theme: p.Theme, Speaker: "you", Body: body, Own: true}}
}

func presentMarkdown(speaker string) func(Presentation, client.Block) []headless.Block {
	return func(p Presentation, block client.Block) []headless.Block {
		message := &markdownMessage{theme: p.Theme, speaker: speaker}
		look := p.Look
		if block.Kind == client.BlockReasoning {
			look.Text, look.Strong = p.Theme.Muted, p.Theme.Subtle
		}
		message.doc.SetBlocks(markdown.Render(block.Text, look))
		return []headless.Block{message}
	}
}

func presentTool(p Presentation, block client.Block) []headless.Block {
	return []headless.Block{newToolBlock(p, block)}
}

func presentNotice(p Presentation, block client.Block) []headless.Block {
	return []headless.Block{kit.Message{Theme: p.Theme, Speaker: "notice", Body: block.Text}}
}

func presentFailure(p Presentation, block client.Block) []headless.Block {
	return []headless.Block{presentError(p.Theme, block.Text)}
}

func validateCommand(command SlashCommand) error {
	switch {
	case strings.TrimSpace(command.Name) == "":
		return errors.New("slash command has no name")
	case strings.ContainsAny(command.Name, " /\t\n"):
		return fmt.Errorf("slash command %q has an invalid name", command.Name)
	case command.Run == nil && command.Execute == nil:
		return fmt.Errorf("slash command %q has no handler", command.Name)
	case command.Run != nil && command.Execute != nil:
		return fmt.Errorf("slash command %q has two handlers", command.Name)
	default:
		return nil
	}
}
