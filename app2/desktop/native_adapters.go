package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsDirectoryPicker struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (picker wailsDirectoryPicker) ChooseDirectory() (string, error) {
	return picker.dialogs.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		ResolvesAliases(true).
		AttachToWindow(picker.window).
		PromptForSingleSelection()
}

type wailsImageSaver struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (saver wailsImageSaver) SaveImage(suggestedName string, contents []byte) (bool, error) {
	destination, err := saver.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(false).
		SetFilename(suggestedName).
		AddFilter("Image Files", "*"+filepath.Ext(suggestedName)).
		AttachToWindow(saver.window).
		PromptForSingleSelection()
	if err != nil {
		return false, fmt.Errorf("open image save dialog: %w", err)
	}
	if destination == "" {
		return false, nil
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, fmt.Errorf("open selected image: %w", err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("write selected image: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close selected image: %w", err)
	}
	return true, nil
}
