// Package sessionartifact safely loads and publishes portable session files.
package sessionartifact

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/fileflow"
	"github.com/spf13/pathologize"

	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
)

const maximumArtifactBytes int64 = 64 << 20

// Store owns the filesystem boundary for session documents. Its zero value is
// ready to use and publishes without overwriting different existing content.
type Store struct{}

func (Store) Publish(workspace, title, requestedName string, document sessiontransfer.Document) (string, error) {
	if err := document.Validate(); err != nil {
		return "", fmt.Errorf("publish session document: %w", err)
	}
	root, err := existingDirectory(workspace)
	if err != nil {
		return "", err
	}
	name, err := documentName(title, requestedName, document.Format())
	if err != nil {
		return "", err
	}
	destination := pathologize.Join(root, name)
	staged, err := stage(root, document.Bytes())
	if err != nil {
		return "", err
	}
	defer os.Remove(staged)
	flow := fileflow.Flow{FindAvailableName: fileflow.FindAvailableNameAuto, NoCreateDirs: true}
	finalPath, err := flow.Move(staged, destination)
	if err != nil {
		return "", fmt.Errorf("publish session document: %w", err)
	}
	return finalPath, nil
}

// Load reads an explicitly selected JSON artifact. Relative paths resolve from
// the active workspace; absolute paths remain valid so users can move sessions
// between projects without first copying them into the destination workspace.
func (Store) Load(workspace, selectedPath string) (sessiontransfer.Document, error) {
	path, err := resolveInputPath(workspace, selectedPath)
	if err != nil {
		return sessiontransfer.Document{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return sessiontransfer.Document{}, fmt.Errorf("open session artifact: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return sessiontransfer.Document{}, fmt.Errorf("inspect session artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return sessiontransfer.Document{}, errors.New("session artifact is not a regular file")
	}
	if info.Size() > maximumArtifactBytes {
		return sessiontransfer.Document{}, fmt.Errorf("session artifact exceeds %d bytes", maximumArtifactBytes)
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumArtifactBytes+1))
	if err != nil {
		return sessiontransfer.Document{}, fmt.Errorf("read session artifact: %w", err)
	}
	if int64(len(body)) > maximumArtifactBytes {
		return sessiontransfer.Document{}, fmt.Errorf("session artifact exceeds %d bytes", maximumArtifactBytes)
	}
	document, err := sessiontransfer.NewDocument(sessiontransfer.JSON, body)
	if err != nil {
		return sessiontransfer.Document{}, fmt.Errorf("read session artifact: %w", err)
	}
	return document, nil
}

func existingDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("session document workspace is empty")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve session document workspace: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve session document workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect session document workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("session document workspace is not a directory")
	}
	return root, nil
}

func resolveInputPath(workspace, selected string) (string, error) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", errors.New("import path is empty")
	}
	if !filepath.IsAbs(selected) {
		root, err := existingDirectory(workspace)
		if err != nil {
			return "", err
		}
		selected = filepath.Join(root, selected)
	}
	absolute, err := filepath.Abs(selected)
	if err != nil {
		return "", fmt.Errorf("resolve session artifact: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve session artifact: %w", err)
	}
	return resolved, nil
}

func documentName(title, requested string, format sessiontransfer.Format) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		stem := pathologize.Clean(strings.TrimSpace(title))
		if stem == "" || stem == "." || stem == "_" {
			stem = "lyra-session"
		}
		requested = stem + format.Extension()
	}
	if strings.ContainsAny(requested, `/\`) || filepath.Base(requested) != requested || requested == "." || requested == ".." {
		return "", errors.New("export name must be a filename, not a path")
	}
	extension := filepath.Ext(requested)
	if extension == "" {
		requested += format.Extension()
	} else if !strings.EqualFold(extension, format.Extension()) {
		return "", fmt.Errorf("export filename must end in %s", format.Extension())
	}
	cleaned := pathologize.Clean(requested)
	if cleaned == "" || cleaned == "." || cleaned == "_" {
		return "", errors.New("export filename is empty after portable normalization")
	}
	return cleaned, nil
}

func stage(root string, body []byte) (path string, err error) {
	temporary, err := os.CreateTemp(root, ".lyra-session-*")
	if err != nil {
		return "", fmt.Errorf("create session document staging file: %w", err)
	}
	path = temporary.Name()
	defer func() {
		if closeErr := temporary.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close session document staging file: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if _, err = temporary.Write(body); err != nil {
		return "", fmt.Errorf("write session document staging file: %w", err)
	}
	if _, err = temporary.Write([]byte("\n")); err != nil {
		return "", fmt.Errorf("terminate session document staging file: %w", err)
	}
	if err = temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync session document staging file: %w", err)
	}
	return path, nil
}
