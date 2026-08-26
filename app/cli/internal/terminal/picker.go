package terminal

import (
	"fmt"
	"image"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// picker is the common interaction behind sessions, models, commands and other
// searchable catalogs. Domain-specific dialogs supply only labels and actions.
type picker[T any] struct {
	theme  kit.Theme
	glyphs kit.Glyphs
	query  kit.Composer
	items  *headless.Filter[T]
	label  func(T) string
	detail func(T) string
	pick   func(T)
	cancel func()
	areas  headless.Snapshot[pickerAreas]

	pointerGesture pickerPointerGesture
}

type pickerAreas struct{ query, list image.Rectangle }

type pickerPointerGesture struct {
	index   int
	active  bool
	dragged bool
}

func (p *pickerPointerGesture) begin(index int) {
	*p = pickerPointerGesture{index: index, active: true}
}

func (p *pickerPointerGesture) drag() bool {
	if !p.active {
		return false
	}
	p.dragged = true
	return true
}

func (p *pickerPointerGesture) release(index int) (pressed int, commit bool) {
	if !p.active {
		return -1, false
	}
	pressed = p.index
	commit = !p.dragged && index == pressed
	p.cancel()
	return pressed, commit
}

func (p *pickerPointerGesture) cancel() { *p = pickerPointerGesture{} }

func newPicker[T any](theme kit.Theme, glyphs kit.Glyphs, placeholder string, label, detail func(T) string, pick func(T)) *picker[T] {
	p := &picker[T]{theme: theme, glyphs: glyphs, label: label, detail: detail, pick: pick}
	p.query = kit.Composer{Theme: theme, Prompt: glyphs.Marker + " ", MaxRows: 1}
	p.query.Editor().Placeholder = placeholder
	p.items = &headless.Filter[T]{Row: p.row}
	p.items.SetText(func(item T) string { return strings.TrimSpace(label(item) + " " + detail(item)) })
	return p
}

func (p *picker[T]) SetItems(items []T) {
	p.pointerGesture.cancel()
	p.items.SetItems(items)
	p.items.SetPattern(p.query.Text())
}

func (p *picker[T]) Current() (T, bool) { return p.items.Current() }

func (p *picker[T]) interruptPointerGesture() { p.pointerGesture.cancel() }

func (p *picker[T]) Reset() {
	p.query.Reset()
	p.items.SetPattern("")
	p.items.Select(0)
	p.pointerGesture.cancel()
}

func (p *picker[T]) Draw(frame headless.Frame) {
	rects := (layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(1)},
	})
	rows := frame.Subs(rects)
	p.areas.Stage(frame, pickerAreas{query: rects[0], list: rects[1]})
	p.query.Draw(rows[0])
	p.items.Draw(rows[1])
	footer := fmt.Sprintf("%d/%d · ↑↓ move · enter select · esc close", p.items.Matched(), p.items.Len())
	kit.Label{Text: footer, Style: p.theme.Subtle, Align: layout.End, Ellipsis: p.glyphs.Ellipsis}.Draw(rows[2].View)
}

func (p *picker[T]) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		p.pointerGesture.cancel()
		if p.handleKey(key, event) {
			return true
		}
	}
	if mouse, ok := event.(input.Mouse); ok {
		return p.handleMouse(mouse)
	}
	if p.query.Handle(event) {
		p.pointerGesture.cancel()
		p.items.SetPattern(p.query.Text())
		return true
	}
	return false
}

func (p *picker[T]) handleKey(key input.Key, event input.Event) bool {
	if key.Code == input.Enter {
		if item, ok := p.items.Current(); ok && p.pick != nil {
			p.pick(item)
		}
		return true
	}
	if key.Code == input.Esc {
		if p.cancel != nil {
			p.cancel()
		}
		return true
	}
	for _, code := range [...]input.Code{input.Up, input.Down, input.PageUp, input.PageDown, input.Home, input.End} {
		if key.Code == code {
			return p.items.Handle(event)
		}
	}
	return false
}

func (p *picker[T]) handleMouse(mouse input.Mouse) bool {
	areas := p.areas.Value()
	if p.pointerGesture.active {
		if mouse.Action == input.MouseDrag || mouse.Action == input.MouseUp {
			mouse.Pos = mouse.Pos.Sub(areas.list.Min)
			return p.handleListMouse(mouse)
		}
		p.pointerGesture.cancel()
	}
	if mouse.Action == input.MouseDown {
		p.pointerGesture.cancel()
	}
	switch {
	case mouse.Pos.In(areas.query):
		mouse.Pos = mouse.Pos.Sub(areas.query.Min)
		if p.query.Handle(mouse) {
			p.items.SetPattern(p.query.Text())
			return true
		}
	case mouse.Pos.In(areas.list):
		mouse.Pos = mouse.Pos.Sub(areas.list.Min)
		return p.handleListMouse(mouse)
	}
	return false
}

func (p *picker[T]) handleListMouse(mouse input.Mouse) bool {
	switch mouse.Action {
	case input.MouseDown:
		if mouse.Button != input.ButtonLeft || !p.items.Handle(mouse) {
			p.pointerGesture.cancel()
			return false
		}
		p.pointerGesture.begin(p.items.Selected())
		return true
	case input.MouseDrag:
		if mouse.Button != input.ButtonLeft || !p.pointerGesture.drag() {
			p.pointerGesture.cancel()
			return false
		}
		p.items.Handle(mouse)
		return true
	case input.MouseUp:
		if mouse.Button != input.ButtonLeft || !p.pointerGesture.active {
			p.pointerGesture.cancel()
			return false
		}
		pressed, commit := p.pointerGesture.release(p.listIndexAt(mouse.Pos.Y))
		if commit && p.items.Selected() == pressed {
			if item, ok := p.items.Current(); ok && p.pick != nil {
				p.pick(item)
			}
		}
		return true
	default:
		return p.items.Handle(mouse)
	}
}

func (p *picker[T]) listIndexAt(row int) int {
	if row < 0 {
		return -1
	}
	index := p.items.Scroll().Offset() + row
	if index >= p.items.Matched() {
		return -1
	}
	return index
}

func (p *picker[T]) Focus(has bool) {
	if !has {
		p.pointerGesture.cancel()
	}
	p.query.Focus(has)
}

func (p *picker[T]) row(view grid.View, _ int, item T, _ fuzzy.Match, selected bool) {
	width, _ := view.Size()
	if width <= 0 {
		return
	}
	base := p.theme.Text
	prefix := "  "
	if selected {
		base = base.Merge(p.theme.Selection)
		view.Fill(view.Bounds(), p.theme.Selection)
		prefix = p.glyphs.Marker + " "
	}
	label := p.label(item)
	detail := strings.TrimSpace(p.detail(item))
	detailWidth := text.Width(detail)
	prefixWidth := text.Width(prefix)
	labelWidth := max(width-prefixWidth, 1)
	if detail != "" && detailWidth+prefixWidth+3 < width {
		labelWidth = width - detailWidth - prefixWidth - 3
		view.Text(width-detailWidth, 0, detail, base.Merge(p.theme.Subtle))
	}
	shown := text.Truncate(label, max(labelWidth, 1), p.glyphs.Ellipsis)
	view.Text(0, 0, prefix, base.Merge(p.theme.Accent))
	view.Text(prefixWidth, 0, shown, base)
}
