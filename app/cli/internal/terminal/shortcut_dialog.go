package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type shortcutRow struct {
	area        string
	bindings    string
	description string
}

type shortcutDefinition struct {
	area        string
	action      keymap.Action
	description string
	keys        *keymap.Map
}

func (a *app) buildShortcutDialog(theme kit.Theme, glyphs kit.Glyphs, applicationKeys *keymap.Map) {
	guideKeys := headless.DefaultScrollKeys()
	for _, binding := range headless.DefaultStackKeys().Keys(headless.Close) {
		guideKeys.Bind(headless.Close, binding...)
	}
	rows := collectShortcutRows(applicationKeys, a.transcript.Keys(), guideKeys)
	table := &kit.Table{
		Theme: theme,
		Columns: []kit.Column{
			{Title: "Area", Size: layout.Measured(4, 12)},
			{Title: "Keys", Size: layout.Measured(8, 28)},
			{Title: "Action", Size: layout.Flex(1)},
		},
		Rows:   len(rows),
		Header: true,
		Gap:    2,
		Cell: func(row, column int) kit.Cell {
			entry := rows[row]
			values := [...]string{entry.area, entry.bindings, entry.description}
			styles := [...]grid.Style{theme.Subtle, theme.Accent, theme.Text}
			return kit.LabelCell(kit.Label{Text: values[column], Style: styles[column], Ellipsis: glyphs.Ellipsis})
		},
	}
	viewport := headless.NewViewport(headless.Static{Of: table})
	viewport.Keys = guideKeys
	viewport.Scroll().Wheel(a.loop.Environment().Wheel())
	a.shortcutViewport = viewport
	a.shortcutDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Shortcuts", Body: viewport,
		Where: layout.Placement{Width: 88, Height: 24}, Keys: guideKeys,
		Hints: []keymap.Action{headless.ScrollUp, headless.ScrollDown, headless.Close},
	})
}

func (a *app) showShortcutDialog() {
	a.shortcutViewport.Scroll().ToTop()
	a.shortcutDialog.Show()
	a.status.note("keyboard shortcuts")
}

func collectShortcutRows(applicationKeys, transcriptKeys, guideKeys *keymap.Map) []shortcutRow {
	definitions := []shortcutDefinition{
		{area: "Guide", action: headless.ScrollUp, description: "scroll this guide up", keys: guideKeys},
		{area: "Guide", action: headless.ScrollDown, description: "scroll this guide down", keys: guideKeys},
		{area: "Guide", action: headless.Close, description: "close this guide", keys: guideKeys},
		{area: "Composer", action: sendPrompt, description: "send prompt or queue follow-up", keys: applicationKeys},
		{area: "Composer", action: insertNewline, description: "insert a newline", keys: applicationKeys},
		{area: "Composer", action: cancelRun, description: "clear draft or cancel active run", keys: applicationKeys},
		{area: "Composer", action: historyPrevious, description: "recall previous prompt", keys: applicationKeys},
		{area: "Composer", action: historyNext, description: "recall next prompt", keys: applicationKeys},
		{area: "Composer", action: editPrompt, description: "edit prompt in external editor", keys: applicationKeys},
		{area: "Composer", action: chooseModel, description: "choose provider and model", keys: applicationKeys},
		{area: "Application", action: commandPalette, description: "open command palette", keys: applicationKeys},
		{area: "Application", action: showShortcuts, description: "open this shortcut guide", keys: applicationKeys},
		{area: "Application", action: showSessions, description: "search and switch sessions", keys: applicationKeys},
		{area: "Application", action: searchTranscript, description: "find text in the live transcript", keys: applicationKeys},
		{area: "Application", action: manageQueue, description: "manage queued follow-ups", keys: applicationKeys},
		{area: "Application", action: toggleDetails, description: "expand or collapse all tool details", keys: applicationKeys},
		{area: "Application", action: nextMatch, description: "go to next search match", keys: applicationKeys},
		{area: "Application", action: previousMatch, description: "go to previous search match", keys: applicationKeys},
		{area: "Application", action: quitApp, description: "quit after confirmation", keys: applicationKeys},
		{area: "Transcript", action: commandPalette, description: "open command palette", keys: transcriptKeys},
		{area: "Transcript", action: scrollPageUp, description: "scroll one page up", keys: applicationKeys},
		{area: "Transcript", action: scrollPageDown, description: "scroll one page down", keys: applicationKeys},
		{area: "Transcript", action: scrollTop, description: "jump to the beginning", keys: applicationKeys},
		{area: "Transcript", action: scrollBottom, description: "jump to the latest output", keys: applicationKeys},
		{area: "Transcript", action: headless.SelectPrev, description: "select previous entry", keys: transcriptKeys},
		{area: "Transcript", action: headless.SelectNext, description: "select next entry", keys: transcriptKeys},
		{area: "Transcript", action: headless.SelectFirst, description: "select first retained entry", keys: transcriptKeys},
		{area: "Transcript", action: headless.SelectLast, description: "select last retained entry", keys: transcriptKeys},
		{area: "Transcript", action: headless.Expand, description: "expand selected tool", keys: transcriptKeys},
		{area: "Transcript", action: headless.Collapse, description: "collapse selected tool", keys: transcriptKeys},
		{area: "Transcript", action: toggleDetails, description: "toggle selected tool", keys: transcriptKeys},
		{area: "Transcript", action: headless.Copy, description: "copy selected entry", keys: transcriptKeys},
		{area: "Transcript", action: transcriptPrompt, description: "return to the composer", keys: transcriptKeys},
	}

	rows := make([]shortcutRow, 0, len(definitions))
	for _, definition := range definitions {
		bindings := formatKeyBindings(definition.keys, definition.action, " / ")
		if bindings == "" {
			continue
		}
		rows = append(rows, shortcutRow{
			area: definition.area, bindings: bindings, description: definition.description,
		})
	}
	return rows
}
