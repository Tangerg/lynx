package terminal

import (
	"context"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

func (a *app) buildReader(theme kit.Theme, glyphs kit.Glyphs) {
	a.reader = newReaderPane(theme, glyphs, a.syntax, a.loop.Environment().Wheel(), a.loop.Clipboard())
	a.readerDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Reader", Body: a.reader,
		Where: layout.Placement{},
	})
	a.reader.dismiss = func() {
		a.reader.CloseDocument()
		a.readerDialog.Dismiss()
	}
	a.reader.openSearch = a.showReaderSearchDialog
	a.reader.onCopied = func() { a.status.note("copied reader text") }

	field := &headless.Text{Label: "Find in the reader", Placeholder: "text", Value: headless.Bind(&a.readerSearchQuery), Check: requiredText}
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		a.readerSearchDialog.Dismiss()
		a.reader.Find(a.readerSearchQuery)
	}
	form.GaveUp = func() { a.readerSearchDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.readerSearchDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Search reader", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.listenForReaderSearch()
}

func (a *app) OpenReader() {
	target, ok := a.transcript.selectedReaderTarget()
	if !ok {
		a.status.note("select a readable transcript entry")
		return
	}
	a.reader.Open(target)
	a.readerDialog.Controller().SetDescription(target.document.Title)
	a.readerDialog.Show()
}

func (a *app) showReaderSearchDialog() {
	a.readerSearchQuery = ""
	a.readerSearchDialog.Show()
}

func (a *app) listenForReaderSearch() {
	results := a.reader.SearchResults()
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(readerSearchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := post(ctx, dispatcher, func() {
					if a.operations.Current(lease) && !a.closed {
						a.reader.AcceptSearch(result)
					}
				}); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

var _ headless.Widget = (*readerPane)(nil)
