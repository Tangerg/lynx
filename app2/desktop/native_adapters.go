package main

import (
	"errors"
	"fmt"
	"io"
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

type wailsSessionDocumentTransfer struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (transfer wailsSessionDocumentTransfer) OpenArtifact(maxBytes int64) ([]byte, bool, error) {
	selected, err := transfer.dialogs.OpenFile().
		CanChooseDirectories(false).
		CanChooseFiles(true).
		ResolvesAliases(true).
		AddFilter("Lyra Session Artifact", "*.json").
		AttachToWindow(transfer.window).
		PromptForSingleSelection()
	if err != nil {
		return nil, false, fmt.Errorf("open session import dialog: %w", err)
	}
	if selected == "" {
		return nil, false, nil
	}
	if !filepath.IsAbs(selected) {
		return nil, false, errors.New("selected session artifact path is not absolute")
	}
	file, err := os.Open(selected)
	if err != nil {
		return nil, false, fmt.Errorf("open selected session artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("inspect selected session artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, false, fmt.Errorf("selected session artifact must be a regular file no larger than %d bytes", maxBytes)
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("read selected session artifact: %w", err)
	}
	if int64(len(contents)) > maxBytes {
		return nil, false, fmt.Errorf("selected session artifact exceeds %d bytes", maxBytes)
	}
	return contents, true, nil
}

func (transfer wailsSessionDocumentTransfer) SaveExport(
	suggestedName string,
	contents []byte,
) (bool, error) {
	extension := filepath.Ext(suggestedName)
	destination, err := transfer.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(false).
		SetFilename(suggestedName).
		AddFilter("Lyra Session Export", "*"+extension).
		AttachToWindow(transfer.window).
		PromptForSingleSelection()
	if err != nil {
		return false, fmt.Errorf("open session export dialog: %w", err)
	}
	if destination == "" {
		return false, nil
	}
	if !filepath.IsAbs(destination) {
		return false, errors.New("session export destination is not absolute")
	}
	if err := writePrivateFileAtomically(filepath.Clean(destination), contents); err != nil {
		return false, err
	}
	return true, nil
}

func writePrivateFileAtomically(destination string, contents []byte) error {
	parent := filepath.Dir(destination)
	candidate, err := os.CreateTemp(parent, ".lyra-session-export-*")
	if err != nil {
		return fmt.Errorf("create session export candidate: %w", err)
	}
	candidatePath := candidate.Name()
	defer os.Remove(candidatePath)
	if err := candidate.Chmod(0o600); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("protect session export candidate: %w", err)
	}
	if _, err := candidate.Write(contents); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("write session export candidate: %w", err)
	}
	if err := candidate.Sync(); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("sync session export candidate: %w", err)
	}
	if err := candidate.Close(); err != nil {
		return fmt.Errorf("close session export candidate: %w", err)
	}
	if err := os.Rename(candidatePath, destination); err != nil {
		return fmt.Errorf("publish selected session export: %w", err)
	}
	return nil
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
