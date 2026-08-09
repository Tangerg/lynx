package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
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
}

// SlashCommand is a contributed composer command.
type SlashCommand struct {
	Name    string
	Title   string
	Aliases []string
	Takes   bool
	Run     func(CommandHost, string) error
}

// Presentation is the terminal vocabulary a block presenter receives.
type Presentation struct {
	Theme  kit.Theme
	Glyphs kit.Glyphs
	Look   markdown.Look
}

// BlockPresenter maps one closed domain block kind to terminal blocks.
type BlockPresenter struct {
	Kind    client.BlockKind
	Present func(Presentation, client.Block) []headless.Block
}

var (
	SlashCommands   = extensions.NewKeyedPoint("terminal.slash-command", func(command SlashCommand) string { return command.Name })
	BlockPresenters = extensions.NewKeyedPoint("terminal.block-presenter", func(presenter BlockPresenter) string { return string(presenter.Kind) })
)

func builtinPlugin() extensions.Plugin {
	return extensions.Plugin{ID: "terminal.core", Setup: func(scope *extensions.Scope) error {
		commands := []SlashCommand{
			{Name: "help", Title: "show commands available in this session", Run: func(host CommandHost, _ string) error { host.ShowHelp(); return nil }},
			{Name: "clear", Title: "release the live transcript", Run: func(host CommandHost, _ string) error { host.Clear(); return nil }},
			{Name: "find", Title: "find text in the live transcript", Takes: true, Run: func(host CommandHost, query string) error { host.Find(query); return nil }},
			{Name: "next", Title: "step to the next search match", Run: func(host CommandHost, _ string) error { host.NextMatch(); return nil }},
			{Name: "previous", Title: "step to the previous search match", Aliases: []string{"prev"}, Run: func(host CommandHost, _ string) error { host.PreviousMatch(); return nil }},
			{Name: "quit", Title: "leave lyra", Aliases: []string{"exit"}, Run: func(host CommandHost, _ string) error { host.Quit(); return nil }},
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
	return []headless.Block{kit.Message{Theme: p.Theme, Speaker: "you", Body: block.Text, Own: true}}
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
	if block.Tool == nil {
		return []headless.Block{presentError(p.Theme, "tool block has no tool projection")}
	}
	tool := block.Tool
	speaker := "tool · " + tool.Name
	body := strings.TrimSpace(tool.Summary)
	if tool.Output != "" {
		body += "\n\n```text\n" + strings.TrimRight(tool.Output, "\n") + "\n```"
	}
	if tool.Diff != "" {
		body += "\n\n```diff\n" + strings.TrimRight(tool.Diff, "\n") + "\n```"
	}
	if tool.Duration > 0 {
		body += fmt.Sprintf("\n\n_%s in %s_", tool.Status, compactTime(tool.Duration))
	}
	message := &markdownMessage{theme: p.Theme, speaker: speaker}
	message.doc.SetBlocks(markdown.Render(strings.TrimSpace(body), p.Look))
	return []headless.Block{message}
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
	case command.Run == nil:
		return fmt.Errorf("slash command %q has no handler", command.Name)
	default:
		return nil
	}
}
