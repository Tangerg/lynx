package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

const (
	saveContextDocument   keymap.Action = "save document"
	cancelContextDocument keymap.Action = "cancel editing"
)

// contextEditor is a reusable, multiline editor for durable context. Its value
// lives in the editor until Save succeeds, so validation or backend failures
// never destroy the user's draft.
type contextEditor struct {
	composer kit.Composer
	theme    kit.Theme
	keys     *keymap.Map
	save     func(string) error
	cancel   func()
	problem  string
	failed   bool
	saving   bool
}

func newContextEditor(theme kit.Theme, clipboard headless.Clipboard, content, placeholder string) *contextEditor {
	keys := headless.DefaultEditorKeys()
	keys.Bind(headless.InsertNewline, input.Chord{Code: input.Enter})
	keys.Bind(saveContextDocument, input.Ctrl.Rune('s'))
	keys.Bind(cancelContextDocument, input.Chord{Code: input.Esc})
	editor := &contextEditor{
		theme: theme, keys: keys,
		composer: kit.Composer{
			Theme: theme, MaxRows: 1_000,
		},
	}
	editor.composer.Editor().Keys = keys
	editor.composer.Editor().Clipboard = clipboard
	editor.composer.Editor().Placeholder = placeholder
	editor.composer.SetText(content)
	return editor
}

func (editor *contextEditor) Draw(frame headless.Frame) {
	_, height := frame.Size()
	if editor.problem == "" || height < 2 {
		editor.composer.Draw(frame)
		return
	}
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Fixed(1)},
	}))
	editor.composer.Draw(rows[0])
	style := editor.theme.Subtle
	if editor.failed {
		style = editor.theme.Danger
	}
	rows[1].Text(0, 0, editor.problem, style)
}

func (editor *contextEditor) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := editor.keys.Action(key.Chord())
		switch action {
		case saveContextDocument:
			if editor.save != nil {
				_ = editor.save(editor.composer.Text())
			}
			return true
		case cancelContextDocument:
			if editor.cancel != nil {
				editor.cancel()
			}
			return true
		}
	}
	return editor.composer.Handle(event)
}

func (editor *contextEditor) Focus(has bool) { editor.composer.Focus(has) }

func (a *app) openContextEditor(title, description, content, placeholder string, save func(string, func(error) bool) error) {
	if a.contextEditorDialog != nil {
		a.contextEditorDialog.Dismiss()
		a.contextEditorDialog = nil
	}
	editor := newContextEditor(a.transcript.theme, a.loop.Clipboard(), content, placeholder)
	var dialog *kit.Dialog
	dismiss := func() {
		if dialog != nil {
			dialog.Dismiss()
		}
		if a.contextEditorDialog == dialog {
			a.contextEditorDialog = nil
		}
	}
	editor.cancel = dismiss
	editor.save = func(value string) error {
		if editor.saving {
			return nil
		}
		editor.saving = true
		editor.problem, editor.failed = "Saving…", false
		if dialog != nil {
			dialog.Controller().SetDescription("Saving…")
		}
		complete := func(err error) bool {
			editor.saving = false
			if err != nil {
				editor.problem, editor.failed = err.Error(), true
				if dialog != nil {
					dialog.Controller().SetDescription(err.Error())
				}
				return false
			}
			if editor.composer.Text() != value {
				editor.problem, editor.failed = "Saved. New edits remain unsaved.", false
				if dialog != nil {
					dialog.Controller().SetDescription(editor.problem)
				}
				return false
			}
			dismiss()
			return true
		}
		if err := save(value, complete); err != nil {
			complete(err)
			a.message(title + ": " + err.Error())
			return err
		}
		return nil
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Description: description, Body: editor,
		Where: layout.Placement{Width: 100, Height: 24}, Keys: editor.keys,
		Hints: []keymap.Action{saveContextDocument, cancelContextDocument},
	})
	a.contextEditorDialog = dialog
	dialog.Show()
}

var _ headless.Widget = (*contextEditor)(nil)
var _ headless.Interactive = (*contextEditor)(nil)
var _ headless.Focusable = (*contextEditor)(nil)
