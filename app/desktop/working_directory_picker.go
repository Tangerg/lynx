package main

import "github.com/wailsapp/wails/v3/pkg/application"

// wailsWorkingDirectoryPicker is the platform adapter for the DesktopHost's
// directory-selection capability. Keeping the concrete DialogManager and Window
// here leaves the host contract independent of Wails' broad application types.
type wailsWorkingDirectoryPicker struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsWorkingDirectoryPicker) ChooseWorkingDirectory() (string, error) {
	return w.dialogs.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		ResolvesAliases(true).
		AttachToWindow(w.window).
		PromptForSingleSelection()
}
