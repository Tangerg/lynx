package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

func (a *app) buildCommandPalette(theme kit.Theme, glyphs kit.Glyphs) {
	a.commandPicker = newPicker(theme, glyphs, "search commands",
		func(command headless.Command) string { return "/" + command.Name },
		func(command headless.Command) string { return command.Title },
		func(command headless.Command) {
			a.commandDialog.Dismiss()
			if command.Takes {
				a.composer.Editor().SetText("/" + command.Name + " ")
				a.composer.Editor().Focus(true)
				return
			}
			a.runCommand(command.Name, "")
		},
	)
	a.commandDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Commands", Body: a.commandPicker,
		Where: layout.Placement{Width: 82, Height: 18, Margin: 1},
	})
	a.commandPicker.cancel = a.commandDialog.Dismiss
}

func (a *app) showCommandPalette() {
	found := a.commands.Find("")
	commands := make([]headless.Command, 0, len(found))
	for _, match := range found {
		commands = append(commands, match.Command)
	}
	a.commandPicker.Reset()
	a.commandPicker.SetItems(commands)
	a.commandDialog.Show()
	a.status.note("choose a command")
}

func (a *app) buildSearchDialog(theme kit.Theme, glyphs kit.Glyphs) {
	field := &headless.Text{Label: "Find in the live transcript", Placeholder: "text", Value: headless.Bind(&a.searchQuery), Check: requiredText}
	keys := headless.DefaultFormKeys()
	form := headless.NewForm(field)
	form.Keys = keys
	form.Done = func() {
		a.searchDialog.Dismiss()
		a.Find(a.searchQuery)
	}
	form.GaveUp = func() { a.searchDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.searchDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Search", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7, Margin: 1},
	})
}

func (a *app) showSearchDialog() {
	a.searchQuery = ""
	a.searchDialog.Show()
}
