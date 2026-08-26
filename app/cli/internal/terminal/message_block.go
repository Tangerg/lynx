package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/highlight"
	"github.com/Tangerg/oolong/markdown"
)

const userMessageInset = 1

// userMessageBlock gives the user's request a quiet surface without turning it
// into a dialog. The transcript still owns scrolling, selection, and retention;
// this block owns only the visual hierarchy of one durable message.
type userMessageBlock struct {
	box     kit.Box
	message *kit.Message
}

var (
	_ headless.Block    = (*userMessageBlock)(nil)
	_ headless.Copyable = (*userMessageBlock)(nil)
)

func newUserMessageBlock(theme kit.Theme, body string) *userMessageBlock {
	return newUserMessageBlockAs(theme, "you", body, true)
}

func newUserMessageBlockAs(theme kit.Theme, speaker, body string, own bool) *userMessageBlock {
	return &userMessageBlock{
		box: kit.Box{
			Theme:   theme,
			Bare:    true,
			Padding: layout.Symmetric(0, userMessageInset),
		},
		message: &kit.Message{Theme: theme, Speaker: speaker, Body: body, Own: own},
	}
}

func (u *userMessageBlock) Measure(width int) int {
	if u == nil {
		return 0
	}
	innerWidth, _ := u.geometry(width)
	return u.message.Measure(innerWidth)
}

func (u *userMessageBlock) Draw(view grid.View) {
	if u == nil {
		return
	}
	width, _ := view.Size()
	if _, inset := u.geometry(width); inset == 0 {
		u.message.Draw(view)
		return
	}
	u.message.Draw(u.box.Draw(view))
}

func (u *userMessageBlock) Rows(width int) []text.Row {
	if u == nil {
		return nil
	}
	innerWidth, inset := u.geometry(width)
	rows := u.message.Rows(innerWidth)
	for index := range rows {
		rows[index].Offset += inset
	}
	return rows
}

func (u *userMessageBlock) geometry(width int) (innerWidth, inset int) {
	overhead := u.box.Overhead().X
	if width <= overhead {
		return max(width, 1), 0
	}
	return width - overhead, userMessageInset
}

type markdownBlock struct {
	theme   kit.Theme
	speaker string
	doc     markdown.Doc
}

func (m *markdownBlock) Measure(width int) int {
	if m == nil {
		return 0
	}
	return layout.Sum(1, m.doc.Measure(max(width-2, 1)), 1)
}

func (m *markdownBlock) Draw(view grid.View) {
	if m == nil {
		return
	}
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	view.Text(0, 0, m.speaker, m.theme.Muted)
	m.doc.Draw(view.Sub(grid.Rect(2, 1, max(width-2, 0), max(height-2, 0))))
}

func (m *markdownBlock) Rows(width int) []text.Row {
	if m == nil {
		return nil
	}
	rows := []text.Row{{Text: m.speaker}}
	for _, row := range m.doc.Rows(max(width-2, 1)) {
		row.Offset += 2
		rows = append(rows, row)
	}
	return append(rows, text.Row{})
}

func markdownLook(theme kit.Theme, glyphs kit.Glyphs, syntax highlight.Renderer) markdown.Look {
	look := markdown.Look{
		Text: theme.Text, Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong: theme.Strong, Emphasis: grid.Style{Attr: grid.Italic},
		Struck: theme.Muted, Code: theme.Info, Block: theme.Sunken,
		Link: theme.Accent, Quote: theme.Muted, Rail: theme.Subtle,
		Marker: theme.Accent, Rule: theme.Divider,
		Glyphs: markdown.Glyphs{
			Bullet: glyphs.Bullet, Bar: glyphs.Vertical, Divider: glyphs.Horizontal,
			Checked: glyphs.Taken, Unchecked: glyphs.Free,
		},
	}
	look.SetRenderer(markdown.FencedCode, syntax.Lines)
	return look
}

func presentError(theme kit.Theme, message string) headless.Block {
	danger := theme
	danger.Text = theme.Danger
	return &kit.Message{Theme: danger, Speaker: "runtime", Body: message}
}
