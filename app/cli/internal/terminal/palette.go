package terminal

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type commandPaletteItem struct {
	command      headless.Command
	category     string
	availability CommandAvailability
}

func (a *app) buildCommandPalette(theme kit.Theme, glyphs kit.Glyphs) {
	a.commandPicker = newPicker(theme, glyphs, "search commands",
		func(item commandPaletteItem) string { return "/" + item.command.Name },
		func(item commandPaletteItem) string {
			detail := item.category + " · " + item.command.Title
			if !item.availability.Enabled {
				detail = item.category + " · unavailable: " + item.availability.Reason
			}
			return detail
		},
		func(item commandPaletteItem) {
			if !a.commandDialog.Open() {
				return
			}
			command := item.command
			a.commandDialog.Dismiss()
			if !item.availability.Enabled {
				a.message("/" + command.Name + " unavailable: " + item.availability.Reason)
				return
			}
			if command.Takes {
				a.composer.Editor().SetText("/" + command.Name + " ")
				a.composer.Editor().Focus(true)
				a.scheduleDraftPersistence()
				return
			}
			a.runCommand(command.Name, "")
		},
	)
	a.commandDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Commands", Body: a.commandPicker,
		Where: layout.Placement{Width: 82, Height: 18},
	})
	a.commandPicker.cancel = a.commandDialog.Dismiss
}

func (a *app) showCommandPalette() {
	found := a.commands.find("")
	commands := make([]commandPaletteItem, 0, len(found))
	for _, match := range found {
		commands = append(commands, commandPaletteItem{
			command: match.Command, category: a.commands.category(match.Command.Name),
			availability: a.commands.availability(match.Command.Name, a),
		})
	}
	slices.SortStableFunc(commands, func(left, right commandPaletteItem) int {
		if compared := commandCategoryRank(left.category) - commandCategoryRank(right.category); compared != 0 {
			return compared
		}
		return strings.Compare(left.command.Name, right.command.Name)
	})
	a.commandPicker.Reset()
	a.commandPicker.SetItems(commands)
	a.commandDialog.Show()
	a.status.note("choose a command")
}

// commandCategoryRank keeps the commands used to navigate and understand the
// current conversation ahead of less frequent configuration commands. The
// category label remains presentation data; ordering is an explicit product
// policy rather than an accidental consequence of English spelling.
func commandCategoryRank(category string) int {
	switch category {
	case commandCategoryApplication:
		return 0
	case commandCategoryTranscript:
		return 1
	case commandCategorySessions:
		return 2
	case commandCategoryComposer:
		return 3
	case commandCategoryRuntime:
		return 4
	case commandCategoryExtensions:
		return 5
	default:
		return 6
	}
}

func (a *app) buildSearchDialog(theme kit.Theme, glyphs kit.Glyphs) {
	field := &headless.Text{Label: "Find in the live transcript", Placeholder: "text", Value: headless.Bind(&a.searchQuery), Check: requiredText}
	keys := headless.DefaultFormKeys()
	form := headless.NewForm(field)
	form.Keys = keys
	form.Done = func() {
		if !a.searchDialog.Open() {
			return
		}
		a.searchDialog.Dismiss()
		a.Find(a.searchQuery)
	}
	form.GaveUp = func() { a.searchDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.searchDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Search", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
}

func (a *app) showSearchDialog() {
	a.searchQuery = ""
	a.searchDialog.Show()
}
