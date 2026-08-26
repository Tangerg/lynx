package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// wailsImageSaver is the platform adapter for DesktopHost's image-save capability.
// It attaches the sheet to the exact window that issued the request, then writes only
// after the user has chosen a destination.
type wailsImageSaver struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsImageSaver) SaveImage(suggestedFilename string, contents []byte) (bool, error) {
	destination, err := w.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(false).
		SetFilename(suggestedFilename).
		AddFilter("Image Files", "*"+filepath.Ext(suggestedFilename)).
		AttachToWindow(w.window).
		PromptForSingleSelection()
	if err != nil {
		return false, fmt.Errorf("open save dialog: %w", err)
	}
	if destination == "" {
		return false, nil
	}
	if err := os.WriteFile(destination, contents, 0o666); err != nil {
		return false, fmt.Errorf("write selected image: %w", err)
	}
	return true, nil
}
