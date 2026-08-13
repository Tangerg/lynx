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
	a.readerDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Reader", Body: a.reader,
		Where: layout.Placement{},
	})
	a.reader.dismiss = a.dismissReader
	a.reader.openSearch = func() {
		if a.readerDialog.Open() {
			a.showReaderSearchDialog()
		}
	}
	a.reader.onCopied = func() { a.status.note("copied reader text") }

	field := &headless.Text{Label: "Find in the reader", Placeholder: "text", Value: headless.Bind(&a.readerSearchQuery), Check: requiredText}
	form := headless.NewForm(field)
	form.Keys = headless.DefaultFormKeys()
	form.Done = func() {
		if !a.readerDialog.Open() || !a.readerSearchDialog.Open() {
			return
		}
		a.readerSearchDialog.Dismiss()
		a.reader.Find(a.readerSearchQuery)
	}
	form.GaveUp = func() { a.readerSearchDialog.Dismiss() }
	dressed := kit.NewForm(kit.FormConfig{
		Theme: theme, Glyphs: glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	a.readerSearchDialog = newPresentationDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Search reader", Body: dressed,
		Where: layout.Placement{Width: 68, Height: 7},
	})
	a.listenForReaderSearch()
}

func (a *app) dismissReader() {
	a.operations.Cancel(readerDocumentOperation)
	a.workspaceReader = workspaceReaderNone
	a.setRuntimeReader(runtimeReaderNone)
	if a.reader != nil {
		a.reader.CloseDocument()
	}
	if a.readerSearchDialog != nil {
		a.readerSearchDialog.Dismiss()
	}
	if a.readerDialog != nil {
		a.readerDialog.Dismiss()
	}
}

func (a *app) OpenReader() {
	target, ok := a.transcript.selectedReaderTarget()
	if !ok {
		a.status.note("select a readable transcript entry")
		return
	}
	a.workspaceReader = workspaceReaderNone
	a.setRuntimeReader(runtimeReaderNone)
	a.openReaderTarget(target)
}

func (a *app) openReaderDocument(document readerDocument) {
	a.openReaderTarget(readerTarget{document: document})
}

func (a *app) openReaderTarget(target readerTarget) {
	a.operations.Cancel(readerDocumentOperation)
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
