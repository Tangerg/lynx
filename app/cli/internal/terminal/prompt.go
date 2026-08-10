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
	theme kit.Theme
	panel *kit.Panel
	help  kit.Help
	rows  *headless.Container
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
		theme: theme,
		panel: panel,
		help:  kit.Help{Theme: theme, Keys: keys, Separator: "  " + glyphs.Vertical + "  "},
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
	if busy {
		p.help.Show = []keymap.Action{cancelRun, insertNewline, toggleDetails, commandPalette}
		return
	}
	p.help.Show = []keymap.Action{sendPrompt, insertNewline, commandPalette, showSessions, cycleMode}
}
