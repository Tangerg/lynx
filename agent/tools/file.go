package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

type readFileArgs struct {
	// Path is relative to the sandbox root.
	Path string `json:"path"`
}

type listFilesArgs struct {
	// Pattern is a glob (e.g. "*.md", "src/**" is NOT supported — use a
	// single path segment glob) resolved within the sandbox root.
	Pattern string `json:"pattern"`
}

// FileTools returns read-only file tools — read_file and list_files —
// sandboxed to dir. Every path is resolved within dir and any attempt to
// escape it (via "..", absolute paths, or symlink-style traversal in the
// cleaned path) is rejected. Exposing unrestricted filesystem reads to an
// LLM is unsafe, so a root is required.
//
// Returns an error when dir is empty or cannot be resolved to an absolute
// path.
func FileTools(dir string) ([]chat.Tool, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("tools.FileTools: dir must not be empty")
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("tools.FileTools: resolve root: %w", err)
	}

	// resolve cleans rel against root and rejects anything escaping it —
	// both absolute paths and "../" traversal that climbs above root.
	resolve := func(rel string) (string, error) {
		if filepath.IsAbs(rel) {
			return "", fmt.Errorf("path %q escapes the sandbox root: absolute paths are not allowed", rel)
		}
		full := filepath.Clean(filepath.Join(root, rel))
		if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
			return "", fmt.Errorf("path %q escapes the sandbox root", rel)
		}
		return full, nil
	}

	readFile, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "read_file",
			Description: "Read a UTF-8 text file by path relative to the sandbox root.",
			InputSchema: pkgjson.MustStringDefSchemaOf(readFileArgs{}),
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			var in readFileArgs
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("read_file: parse arguments: %w", err)
			}
			full, err := resolve(in.Path)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}
			return string(data), nil
		},
	)

	listFiles, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "list_files",
			Description: "List files matching a glob pattern relative to the sandbox root; returns newline-separated relative paths.",
			InputSchema: pkgjson.MustStringDefSchemaOf(listFilesArgs{}),
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			var in listFilesArgs
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("list_files: parse arguments: %w", err)
			}
			pattern := in.Pattern
			if pattern == "" {
				pattern = "*"
			}
			matches, err := filepath.Glob(filepath.Join(root, pattern))
			if err != nil {
				return "", fmt.Errorf("list_files: %w", err)
			}

			var rels []string
			for _, m := range matches {
				clean := filepath.Clean(m)
				if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
					continue // defensive: drop anything outside root
				}
				rel, err := filepath.Rel(root, clean)
				if err != nil {
					continue
				}
				rels = append(rels, rel)
			}
			slices.Sort(rels)
			return strings.Join(rels, "\n"), nil
		},
	)

	return []chat.Tool{readFile, listFiles}, nil
}
