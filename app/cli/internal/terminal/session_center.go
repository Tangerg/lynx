package terminal

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type sessionCenterPane struct {
	theme          kit.Theme
	glyphs         kit.Glyphs
	picker         *picker[agent.Session]
	items          []agent.Session
	cursor         string
	seenCursors    map[string]struct{}
	loadMore       func()
	toggleFavorite func(agent.Session)
	rename         func(agent.Session)
	delete         func(agent.Session)
}

func newSessionCenterPane(theme kit.Theme, glyphs kit.Glyphs, open func(agent.Session)) *sessionCenterPane {
	center := &sessionCenterPane{theme: theme, glyphs: glyphs}
	center.picker = newPicker(theme, glyphs, "search loaded sessions",
		func(session agent.Session) string {
			group := "Recent"
			if session.Favorite {
				group = glyphs.Taken + " Favorites"
			}
			return group + " · " + displayTitle(session)
		},
		func(session agent.Session) string { return compactRelativeAge(session.UpdatedAt) },
		open,
	)
	center.Reset()
	return center
}

func (c *sessionCenterPane) Reset() {
	c.items, c.cursor = nil, ""
	c.seenCursors = map[string]struct{}{"": {}}
	c.picker.Reset()
	c.picker.SetItems(nil)
}

func (c *sessionCenterPane) SetPage(page agent.SessionPage, appendPage bool) error {
	if !appendPage {
		c.seenCursors = map[string]struct{}{"": {}}
	}
	if page.NextCursor != "" {
		if _, exists := c.seenCursors[page.NextCursor]; exists {
			return fmt.Errorf("session catalog returned cyclic continuation cursor %q", page.NextCursor)
		}
	}
	next := slices.Clone(page.Items)
	if appendPage {
		seen := make(map[string]struct{}, len(c.items)+len(page.Items))
		for _, session := range c.items {
			seen[session.ID] = struct{}{}
		}
		for _, session := range page.Items {
			if _, duplicate := seen[session.ID]; duplicate {
				return fmt.Errorf("session page repeats previously loaded id %q", session.ID)
			}
			seen[session.ID] = struct{}{}
		}
		next = append(slices.Clone(c.items), page.Items...)
	}
	c.items, c.cursor = sortSessionCenter(next), page.NextCursor
	if page.NextCursor != "" {
		c.seenCursors[page.NextCursor] = struct{}{}
	}
	c.picker.SetItems(c.items)
	return nil
}

func (c *sessionCenterPane) Upsert(session agent.Session) {
	selected, selectedOK := c.picker.Current()
	updated := false
	for index := range c.items {
		if c.items[index].ID == session.ID {
			c.items[index], updated = session, true
			break
		}
	}
	if !updated {
		c.items = append(c.items, session)
	}
	c.items = sortSessionCenter(c.items)
	if selectedOK && selected.ID == session.ID {
		c.picker.Reset()
	}
	c.picker.SetItems(c.items)
}

func (c *sessionCenterPane) Remove(id string) {
	c.items = slices.DeleteFunc(c.items, func(session agent.Session) bool { return session.ID == id })
	c.picker.SetItems(c.items)
}

func (c *sessionCenterPane) HasMore() bool { return c.cursor != "" }

func (c *sessionCenterPane) Cursor() string { return c.cursor }

func (c *sessionCenterPane) Draw(frame headless.Frame) {
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)},
		{Size: layout.Fixed(4)},
		{Size: layout.Fixed(1)},
	}))
	c.picker.Draw(rows[0])
	c.drawPreview(rows[1].View)
	more := "all sessions loaded"
	if c.HasMore() {
		more = "alt+l load more"
	}
	help := "alt+f favorite · alt+r rename · alt+d delete · " + more
	kit.Label{Text: help, Style: c.theme.Subtle, Ellipsis: c.glyphs.Ellipsis}.Draw(rows[2].View)
}

func (c *sessionCenterPane) drawPreview(view grid.View) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	session, ok := c.picker.Current()
	if !ok {
		view.Text(0, 1, "No loaded sessions", c.theme.Muted)
		return
	}
	view.Text(0, 0, text.Truncate(displayTitle(session), width, c.glyphs.Ellipsis), c.theme.Strong)
	view.Text(0, 1, text.Truncate(displayWorkspace(session.Workspace), width, c.glyphs.Ellipsis), c.theme.Context)
	detail := strings.TrimSpace(session.Model)
	if detail != "" {
		detail += " · "
	}
	detail += string(session.Status) + " · updated " + compactRelativeAge(session.UpdatedAt)
	view.Text(0, 2, text.Truncate(detail, width, c.glyphs.Ellipsis), c.theme.Subtle)
	view.Text(0, 3, text.Truncate(session.ID, width, c.glyphs.Ellipsis), c.theme.Muted)
}

func (c *sessionCenterPane) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() && key.Mods == input.Alt {
		c.picker.interruptPointerGesture()
		session, selected := c.picker.Current()
		switch key.Rune {
		case 'l':
			if c.HasMore() && c.loadMore != nil {
				c.loadMore()
			}
			return true
		case 'f':
			if selected && c.toggleFavorite != nil {
				c.toggleFavorite(session)
			}
			return true
		case 'r':
			if selected && c.rename != nil {
				c.rename(session)
			}
			return true
		case 'd':
			if selected && c.delete != nil {
				c.delete(session)
			}
			return true
		}
	}
	return c.picker.Handle(event)
}

func (c *sessionCenterPane) Focus(has bool) { c.picker.Focus(has) }

func sortSessionCenter(sessions []agent.Session) []agent.Session {
	sorted := slices.Clone(sessions)
	slices.SortStableFunc(sorted, func(left, right agent.Session) int {
		if left.Favorite != right.Favorite {
			if left.Favorite {
				return -1
			}
			return 1
		}
		if compared := right.UpdatedAt.Compare(left.UpdatedAt); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	return sorted
}
