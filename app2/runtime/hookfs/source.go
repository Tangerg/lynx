// Package hookfs discovers lifecycle-hook configuration through confined local
// roots. It never decides whether project-authored hooks are trusted to run.
package hookfs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	hookFileRelative = ".lyra/hooks.json"
	maxHookFileBytes = 256 << 10
)

type Source struct {
	home string
}

func New(home string) (*Source, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("hookfs: home must be absolute")
	}
	physical, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil, fmt.Errorf("hookfs: resolve home: %w", err)
	}
	physical, err = filepath.Abs(physical)
	if err != nil {
		return nil, err
	}
	return &Source{home: filepath.Clean(physical)}, nil
}

func (source *Source) Global(
	ctx context.Context,
) ([]lifecyclehook.Hook, error) {
	root, err := os.OpenRoot(source.home)
	if err != nil {
		return nil, fmt.Errorf("hookfs: open home root: %w", err)
	}
	defer root.Close()
	values, _, err := readFile(
		ctx,
		root,
		source.home,
		hookFileRelative,
		lifecyclehook.ScopeGlobal,
	)
	return values, err
}

func (source *Source) Project(
	ctx context.Context,
	target lifecyclehook.Target,
) ([]lifecyclehook.Hook, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	directories, err := rootToLeaf(target.ProjectRoot, target.Workspace)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(target.ProjectRoot)
	if err != nil {
		return nil, fmt.Errorf("hookfs: open project root: %w", err)
	}
	defer root.Close()
	values := make([]lifecyclehook.Hook, 0)
	seen := make([]fs.FileInfo, 0)
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(target.ProjectRoot, directory)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(relative, hookFileRelative)
		hooks, info, err := readFile(
			ctx,
			root,
			target.ProjectRoot,
			path,
			lifecyclehook.ScopeProject,
		)
		if err != nil {
			return nil, err
		}
		if info == nil || slices.ContainsFunc(seen, func(existing fs.FileInfo) bool {
			return os.SameFile(existing, info)
		}) {
			continue
		}
		seen = append(seen, info)
		if len(values)+len(hooks) > lifecyclehook.MaxHooksPerRun {
			return nil, fmt.Errorf(
				"hookfs: project hook cascade exceeds %d entries",
				lifecyclehook.MaxHooksPerRun,
			)
		}
		values = append(values, hooks...)
	}
	return values, nil
}

func (source *Source) Files(
	ctx context.Context,
	projects []lifecyclehook.Target,
) ([]string, error) {
	values := make([]string, 0)
	if err := appendObservedPath(
		&values,
		filepath.Join(source.home, hookFileRelative),
		source.home,
	); err != nil {
		return nil, err
	}
	for _, project := range projects {
		if err := project.Validate(); err != nil {
			return nil, err
		}
		directories, err := rootToLeaf(project.ProjectRoot, project.Workspace)
		if err != nil {
			return nil, err
		}
		for _, directory := range directories {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := appendObservedPath(
				&values,
				filepath.Join(directory, hookFileRelative),
				project.ProjectRoot,
			); err != nil {
				return nil, err
			}
		}
	}
	slices.Sort(values)
	return slices.Compact(values), nil
}

func appendObservedPath(values *[]string, path string, anchor string) error {
	path = filepath.Clean(path)
	physical, err := physicalObservedPath(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(anchor, physical)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return protocol.ErrPathOutsideRoot
	}
	*values = append(*values, path)
	*values = append(*values, physical)
	return nil
}

// physicalObservedPath resolves every existing ancestor and preserves the
// not-yet-created suffix. That keeps future observation confined even when an
// intermediate path is a symlink and the hooks file itself does not exist.
func physicalObservedPath(path string) (string, error) {
	for existing := path; ; existing = filepath.Dir(existing) {
		physical, err := filepath.EvalSymlinks(existing)
		if err == nil {
			suffix, err := filepath.Rel(existing, path)
			if err != nil {
				return "", err
			}
			return filepath.Clean(filepath.Join(physical, suffix)), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(existing) == existing {
			return "", fmt.Errorf("hookfs: %s has no existing ancestor", path)
		}
	}
}

type fileWire struct {
	Hooks *[]hookWire `json:"hooks"`
}

type hookWire struct {
	Event         lifecyclehook.Event `json:"event"`
	Matcher       string              `json:"matcher,omitempty"`
	Command       string              `json:"command,omitempty"`
	Inject        string              `json:"inject,omitempty"`
	TimeoutMillis int                 `json:"timeoutMillis,omitempty"`
}

func readFile(
	ctx context.Context,
	root *os.Root,
	anchor string,
	relative string,
	scope lifecyclehook.Scope,
) ([]lifecyclehook.Hook, fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	file, err := root.Open(relative)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("hookfs: open %s: %w", relative, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("hookfs: %s is not a regular file", relative)
	}
	if info.Size() > maxHookFileBytes {
		return nil, nil, fmt.Errorf("hookfs: %s exceeds %d bytes", relative, maxHookFileBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxHookFileBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxHookFileBytes {
		return nil, nil, fmt.Errorf("hookfs: %s exceeds %d bytes", relative, maxHookFileBytes)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, info, nil
	}
	if trimmed[0] != '{' {
		return nil, nil, fmt.Errorf("hookfs: %s must contain a JSON object", relative)
	}
	var wire fileWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return nil, nil, fmt.Errorf("hookfs: decode %s: %w", relative, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return nil, nil, fmt.Errorf("hookfs: decode %s: %w", relative, err)
	}
	if wire.Hooks == nil {
		return nil, nil, fmt.Errorf("hookfs: %s requires a hooks array", relative)
	}
	if len(*wire.Hooks) > lifecyclehook.MaxHooksPerFile {
		return nil, nil, fmt.Errorf(
			"hookfs: %s exceeds %d hooks",
			relative,
			lifecyclehook.MaxHooksPerFile,
		)
	}
	path := filepath.Join(anchor, filepath.Clean(relative))
	values := make([]lifecyclehook.Hook, len(*wire.Hooks))
	for index, value := range *wire.Hooks {
		values[index] = lifecyclehook.Hook{
			Event: value.Event, Matcher: value.Matcher,
			Command: value.Command, Inject: value.Inject,
			TimeoutMillis: value.TimeoutMillis, Scope: scope, Source: path,
		}
		if err := values[index].Validate(); err != nil {
			return nil, nil, fmt.Errorf("hookfs: %s hook %d: %w", relative, index, err)
		}
	}
	return values, info, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func rootToLeaf(root string, leaf string) ([]string, error) {
	root = filepath.Clean(root)
	leaf = filepath.Clean(leaf)
	if !filepath.IsAbs(root) || !filepath.IsAbs(leaf) {
		return nil, protocol.ErrPathOutsideRoot
	}
	relative, err := filepath.Rel(root, leaf)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, protocol.ErrPathOutsideRoot
	}
	values := make([]string, 0)
	for current := leaf; ; current = filepath.Dir(current) {
		values = append(values, current)
		if current == root {
			break
		}
	}
	slices.Reverse(values)
	return values, nil
}
