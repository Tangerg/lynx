// Package sessionexport renders and safely writes portable CLI session reports.
package sessionexport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/fileflow"
	"github.com/spf13/pathologize"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type Format string

const (
	Markdown Format = "markdown"
	JSON     Format = "json"
)

func ParseFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown", "md":
		return Markdown, nil
	case "json":
		return JSON, nil
	default:
		return "", fmt.Errorf("export format %q is unsupported; use markdown or json", strings.TrimSpace(value))
	}
}

func (f Format) Extension() string {
	switch f {
	case Markdown:
		return ".md"
	case JSON:
		return ".json"
	default:
		return ""
	}
}

// SessionExport is an immutable, validated report ready to render or save.
type SessionExport struct {
	format   Format
	snapshot agent.SessionSnapshot
}

func New(snapshot agent.SessionSnapshot, format Format) (SessionExport, error) {
	if format != Markdown && format != JSON {
		return SessionExport{}, fmt.Errorf("export format %q is unsupported", format)
	}
	if err := snapshot.Validate(); err != nil {
		return SessionExport{}, fmt.Errorf("export session: %w", err)
	}
	return SessionExport{format: format, snapshot: cloneSnapshot(snapshot)}, nil
}

func (e SessionExport) Bytes() ([]byte, error) {
	switch e.format {
	case Markdown:
		return []byte(renderMarkdown(e.snapshot)), nil
	case JSON:
		data, err := json.MarshalIndent(toJSONDocument(e.snapshot), "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode session export: %w", err)
		}
		return append(data, '\n'), nil
	default:
		return nil, fmt.Errorf("export format %q is unsupported", e.format)
	}
}

// Save writes into the trusted workspace root. requestedName is a filename,
// never a path; unsafe characters are normalized portably and existing files
// with different content receive a conflict-safe suffix.
func (e SessionExport) Save(workspace, requestedName string) (string, error) {
	root, err := existingDirectory(workspace)
	if err != nil {
		return "", err
	}
	name, err := e.filename(requestedName)
	if err != nil {
		return "", err
	}
	destination := pathologize.Join(root, name)
	if err := containedBy(root, destination); err != nil {
		return "", err
	}
	data, err := e.Bytes()
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(root, ".lyra-export-*")
	if err != nil {
		return "", fmt.Errorf("create export staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return "", fmt.Errorf("write export staging file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", fmt.Errorf("sync export staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close export staging file: %w", err)
	}
	flow := fileflow.Flow{FindAvailableName: fileflow.FindAvailableNameAuto, NoCreateDirs: true}
	finalPath, err := flow.Move(temporaryPath, destination)
	if err != nil {
		return "", fmt.Errorf("publish session export: %w", err)
	}
	return finalPath, nil
}

func (e SessionExport) filename(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		stem := pathologize.Clean(strings.TrimSpace(e.snapshot.Session.Title))
		if stem == "" || stem == "." || stem == "_" {
			stem = "lyra-session"
		}
		requested = stem + e.format.Extension()
	}
	if strings.ContainsAny(requested, `/\`) || filepath.Base(requested) != requested || requested == "." || requested == ".." {
		return "", errors.New("export name must be a filename, not a path")
	}
	extension := filepath.Ext(requested)
	if extension == "" {
		requested += e.format.Extension()
	} else if !strings.EqualFold(extension, e.format.Extension()) {
		return "", fmt.Errorf("export filename must end in %s", e.format.Extension())
	}
	cleaned := pathologize.Clean(requested)
	if cleaned == "" || cleaned == "." || cleaned == "_" {
		return "", errors.New("export filename is empty after portable normalization")
	}
	return cleaned, nil
}

func LastAssistantText(snapshot agent.SessionSnapshot) (string, error) {
	for _, block := range slices.Backward(snapshot.Transcript) {
		if block.Kind == agent.BlockAssistant && strings.TrimSpace(block.Text) != "" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "", errors.New("the session has no assistant response to copy")
}

func existingDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("export workspace is empty")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve export workspace: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve export workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect export workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("export workspace is not a directory")
	}
	return root, nil
}

func containedBy(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("validate export destination: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("export destination escapes the workspace")
	}
	return nil
}

func cloneSnapshot(snapshot agent.SessionSnapshot) agent.SessionSnapshot {
	cloned := snapshot
	cloned.Transcript = make([]agent.Block, len(snapshot.Transcript))
	for index, block := range snapshot.Transcript {
		cloned.Transcript[index] = block.Clone()
	}
	cloned.Runs = make([]agent.Run, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		cloned.Runs[index] = run.Clone()
	}
	cloned.Plan = slices.Clone(snapshot.Plan)
	cloned.Interactions = agent.CloneInteractions(snapshot.Interactions)
	return cloned
}
