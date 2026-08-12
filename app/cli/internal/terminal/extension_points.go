package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/extensions"
)

// BlockPresentation is the terminal vocabulary a block presenter receives.
type BlockPresentation struct {
	Theme   kit.Theme
	Glyphs  kit.Glyphs
	Look    markdown.Look
	Syntax  highlight.Renderer
	Tools   []ToolPresenter
	Speaker string
	Image   func(agent.InlineImage) headless.Block
}

// BlockPresenter maps one closed domain block kind to terminal blocks.
type BlockPresenter struct {
	Kind    agent.BlockKind
	Present func(BlockPresentation, agent.Block) []headless.Block
}

// CustomEventPresenter gives runtime extensions an explicit projection seam.
// Unhandled custom events remain observable through NDJSON but do not add
// terminal noise; a matching presenter may turn one into ordinary transcript
// blocks without changing the terminal event switch.
type CustomEventPresenter struct {
	Name    string
	Present func(BlockPresentation, agent.CustomEvent) []headless.Block
}

var (
	SlashCommands         = extensions.NewCapabilityKeyedPoint("terminal.slash-command", extensions.Capability("terminal.commands"), func(command SlashCommand) string { return command.Descriptor.Name })
	BlockPresenters       = extensions.NewCapabilityKeyedPoint("terminal.block-presenter", extensions.Capability("terminal.presentation"), func(presenter BlockPresenter) string { return string(presenter.Kind) })
	ToolPresenters        = extensions.NewCapabilityMultiPoint[ToolPresenter]("terminal.tool-presenter", extensions.Capability("terminal.presentation"))
	CustomEventPresenters = extensions.NewCapabilityKeyedPoint("terminal.custom-event-presenter", extensions.Capability("terminal.presentation"), func(presenter CustomEventPresenter) string { return presenter.Name })
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
		for i, presenter := range defaultToolPresenters() {
			if _, err := extensions.Contribute(scope, ToolPresenters, presenter, extensions.Contribution{Order: i}); err != nil {
				return err
			}
		}
		return nil
	}}
}
