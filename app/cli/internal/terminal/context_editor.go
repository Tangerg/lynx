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

type contextEditorRequest struct {
	Title       string
	Description string
	Content     string
	Placeholder string
	Save        func(string, func(error) bool) error
	Dismissed   func()
}

type contextEditorSession struct {
	dialog    *kit.Dialog
	editor    *contextEditor
	dismissed func()
	closed    bool
}

func (c *contextEditorSession) Dismiss() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	if c.dialog != nil {
		c.dialog.Dismiss()
	}
	if c.dismissed != nil {
		c.dismissed()
	}
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

func (c *contextEditor) Draw(frame headless.Frame) {
	_, height := frame.Size()
	if c.problem == "" || height < 2 {
		c.composer.Draw(frame)
		return
	}
	rows := frame.Subs((layout.Flow{Axis: layout.Down}).Rects(frame.Bounds().Size(), []layout.Slot{
		{Size: layout.Flex(1)}, {Size: layout.Fixed(1)},
	}))
	c.composer.Draw(rows[0])
	style := c.theme.Subtle
	if c.failed {
		style = c.theme.Danger
	}
	rows[1].Text(0, 0, c.problem, style)
}

func (c *contextEditor) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok && key.Down() {
		action, _ := c.keys.Action(key.Chord())
		switch action {
		case saveContextDocument:
			if c.save != nil {
				_ = c.save(c.composer.Text())
			}
			return true
		case cancelContextDocument:
			if c.cancel != nil {
				c.cancel()
			}
			return true
		}
	}
	return c.composer.Handle(event)
}

func (c *contextEditor) Focus(has bool) { c.composer.Focus(has) }

func (a *app) openContextEditor(request contextEditorRequest) *contextEditorSession {
	if a.activeContextEditor != nil {
		a.activeContextEditor.Dismiss()
	}
	editor := newContextEditor(a.transcript.theme, a.loop.Clipboard(), request.Content, request.Placeholder)
	var dialog *kit.Dialog
	session := &contextEditorSession{editor: editor}
	session.dismissed = func() {
		if a.activeContextEditor == session {
			a.activeContextEditor = nil
		}
		if request.Dismissed != nil {
			request.Dismissed()
		}
	}
	editor.cancel = session.Dismiss
	editor.save = func(value string) error {
		if editor.saving || session.closed || a.activeContextEditor != session {
			return nil
		}
		editor.saving = true
		editor.problem, editor.failed = "Saving…", false
		if dialog != nil {
			dialog.Controller().SetDescription("Saving…")
		}
		complete := func(err error) bool {
			if session.closed || a.activeContextEditor != session {
				return false
			}
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
			session.Dismiss()
			return true
		}
		if err := request.Save(value, complete); err != nil {
			complete(err)
			a.message(request.Title + ": " + err.Error())
			return err
		}
		return nil
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: request.Title, Description: request.Description, Body: editor,
		Where: layout.Placement{Width: 100, Height: 24}, Keys: editor.keys,
		Hints: []keymap.Action{saveContextDocument, cancelContextDocument},
	})
	session.dialog = dialog
	a.activeContextEditor = session
	dialog.Show()
	return session
}

func (a *app) dismissContextEditor() {
	if a.activeContextEditor != nil {
		a.activeContextEditor.Dismiss()
	}
}

var _ headless.Widget = (*contextEditor)(nil)
var _ headless.Interactive = (*contextEditor)(nil)
var _ headless.Focusable = (*contextEditor)(nil)
