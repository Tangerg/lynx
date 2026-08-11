package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type promptView struct {
	theme             kit.Theme
	panel             *kit.Panel
	composer          *kit.Composer
	help              kit.Help
	rows              *headless.Container
	keys              *keymap.Map
	busyKeys          *keymap.Map
	busyQueuedKeys    *keymap.Map
	busy              bool
	queued            int
	transcriptFocused bool
	selection         transcriptSelection
	transcriptKeys    *keymap.Map
	focused           bool
	compact           bool
}

func newPromptView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	keys *keymap.Map,
	composer *kit.Composer,
	options agent.RunOptions,
) *promptView {
	panel := kit.NewPanel(theme, glyphs, composer)
	panel.Box.Padding = layout.Symmetric(0, 1)
	panel.Box.FooterAlign = layout.End
	p := &promptView{
		theme:          theme,
		panel:          panel,
		composer:       composer,
		help:           kit.Help{Theme: theme, Keys: keys, Separator: "  " + glyphs.Vertical + "  "},
		keys:           keys,
		busyKeys:       remapHelpAction(keys, sendPrompt, queueFollowUp),
		busyQueuedKeys: remapHelpAction(keys, sendPrompt, queueOrSendNext),
	}
	p.rows = headless.NewContainer(layout.Down,
		headless.Item{Key: "field", Size: layout.Measured(3, 8), Of: panel},
		headless.Item{Key: "help", Size: layout.Fixed(1), Of: headless.Static{Of: &p.help}},
	)
	p.rows.Focus(false)
	p.SetOptions(options)
	p.SetBusy(false)
	return p
}

func (p *promptView) Draw(frame headless.Frame) {
	if p.compact {
		p.composer.Draw(frame)
		return
	}
	p.rows.Draw(frame)
}

func (p *promptView) Handle(event input.Event) bool {
	if p.compact {
		return p.composer.Handle(event)
	}
	return p.rows.Handle(event)
}

func (p *promptView) Focus(has bool) {
	p.focused = has
	p.panel.Box.Theme = p.theme
	if has {
		p.panel.Box.Theme.Border = p.theme.Accent
	}
	p.syncFocus()
}

func (p *promptView) Measure(width int) int {
	if p.compact {
		return 1
	}
	return p.rows.Measure(width)
}

func (p *promptView) SetCompact(compact bool) {
	if p.compact == compact {
		return
	}
	p.compact = compact
	p.syncFocus()
}

func (p *promptView) syncFocus() {
	if p.compact {
		p.rows.Focus(false)
		p.composer.Focus(p.focused)
		return
	}
	p.composer.Focus(false)
	p.rows.Focus(p.focused)
}

func (p *promptView) SetOptions(options agent.RunOptions) {
	p.panel.Box.Footer = optionsLabel(options)
}

func (p *promptView) SetPendingKeySequence(hint string) {
	p.panel.Box.Title = hint
}

func (p *promptView) SetBusy(busy bool) {
	p.busy = busy
	p.refreshHelp()
}

func (p *promptView) SetQueued(count int) {
	p.queued = max(count, 0)
	p.refreshHelp()
}

func (p *promptView) SetTranscriptFocused(focused bool) {
	p.transcriptFocused = focused
	p.refreshHelp()
}

func (p *promptView) SetTranscriptSelection(selection transcriptSelection) {
	p.selection = selection
	p.refreshHelp()
}

func (p *promptView) SetTranscriptKeys(keys *keymap.Map) {
	p.transcriptKeys = keys
	p.refreshHelp()
}

func (p *promptView) refreshHelp() {
	p.help.Keys = p.keys
	switch {
	case p.transcriptFocused:
		p.showTranscriptHelp()
		return
	case p.busy:
		p.showBusyHelp()
		return
	}
	p.help.Show = []keymap.Action{sendPrompt, insertNewline, editPrompt, commandPalette, showSessions, chooseModel}
	if p.queued > 0 {
		p.help.Show = append(p.help.Show, manageQueue)
	}
}

func (p *promptView) showTranscriptHelp() {
	p.help.Keys = p.transcriptKeys
	if !p.selection.Present {
		p.help.Show = []keymap.Action{transcriptPrompt}
		return
	}
	p.help.Show = []keymap.Action{headless.SelectPrev, headless.SelectNext, transcriptPrompt}
	if p.selection.Expandable {
		action := headless.Expand
		if p.selection.Expanded {
			action = headless.Collapse
		}
		p.help.Show = append(p.help.Show, action, toggleDetails)
	}
	if p.selection.Readable {
		p.help.Show = append(p.help.Show, openReader)
	}
	p.help.Show = append(p.help.Show, headless.Copy)
}

func (p *promptView) showBusyHelp() {
	p.help.Keys = p.busyKeys
	p.help.Show = []keymap.Action{queueFollowUp, cancelRun, insertNewline, toggleDetails}
	if p.queued == 0 {
		return
	}
	p.help.Keys = p.busyQueuedKeys
	p.help.Show = []keymap.Action{queueOrSendNext, cancelRun, insertNewline, manageQueue, toggleDetails}
}

func remapHelpAction(keys *keymap.Map, from, to keymap.Action) *keymap.Map {
	remapped := &keymap.Map{}
	for _, binding := range keys.Bindings() {
		action := binding.Action
		if action == from {
			action = to
		}
		remapped.Bind(action, binding.Keys...)
	}
	return remapped
}
