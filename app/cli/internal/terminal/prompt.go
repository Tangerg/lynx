package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

type promptView struct {
	theme             kit.Theme
	panel             *kit.Panel
	help              kit.Help
	rows              *headless.Container
	keys              *keymap.Map
	busyKeys          *keymap.Map
	busy              bool
	transcriptFocused bool
	selection         transcriptSelection
	transcriptKeys    *keymap.Map
}

func newPromptView(
	theme kit.Theme,
	glyphs kit.Glyphs,
	keys *keymap.Map,
	composer *kit.Composer,
	options client.RunOptions,
) *promptView {
	panel := kit.NewPanel(theme, glyphs, composer)
	panel.Box.Padding = layout.Symmetric(0, 1)
	panel.Box.FooterAlign = layout.End
	p := &promptView{
		theme:    theme,
		panel:    panel,
		help:     kit.Help{Theme: theme, Keys: keys, Separator: "  " + glyphs.Vertical + "  "},
		keys:     keys,
		busyKeys: remapHelpAction(keys, sendPrompt, queueFollowUp),
	}
	p.rows = headless.Rows(
		headless.Item{Key: "field", Size: layout.Measured(3, 8), Of: panel},
		headless.Item{Key: "help", Size: layout.Fixed(1), Of: headless.Static{Of: &p.help}},
	)
	p.rows.Focus(true)
	p.SetOptions(options)
	p.SetBusy(false)
	return p
}

func (p *promptView) Draw(frame headless.Frame) { p.rows.Draw(frame) }

func (p *promptView) Handle(event input.Event) bool { return p.rows.Handle(event) }

func (p *promptView) Focus(has bool) {
	p.panel.Box.Theme = p.theme
	if has {
		p.panel.Box.Theme.Border = p.theme.Accent
	}
	p.rows.Focus(has)
}

func (p *promptView) Measure(width int) int { return p.rows.Measure(width) }

func (p *promptView) SetOptions(options client.RunOptions) {
	p.panel.Box.Footer = optionsLabel(options)
}

func (p *promptView) SetBusy(busy bool) {
	p.busy = busy
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
	if p.transcriptFocused {
		p.help.Keys = p.transcriptKeys
		if !p.selection.Present {
			p.help.Show = []keymap.Action{transcriptPrompt}
			return
		}
		p.help.Show = []keymap.Action{headless.SelectPrev, headless.SelectNext, transcriptPrompt}
		if p.selection.Expandable {
			if p.selection.Expanded {
				p.help.Show = append(p.help.Show, headless.Collapse)
			} else {
				p.help.Show = append(p.help.Show, headless.Expand)
			}
			p.help.Show = append(p.help.Show, toggleDetails)
		}
		p.help.Show = append(p.help.Show, headless.Copy)
		return
	}
	if p.busy {
		p.help.Keys = p.busyKeys
		p.help.Show = []keymap.Action{queueFollowUp, cancelRun, insertNewline, toggleDetails}
		return
	}
	p.help.Show = []keymap.Action{sendPrompt, insertNewline, commandPalette, showSessions, cycleMode}
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
