// Package workspaceflow owns safe filesystem and Git reads scoped to one
// explicit Workspace. Paths are jailed after symlink resolution and returned as
// plain text/rows; renderer markup is never produced here.
package workspaceflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

const (
	defaultReadBytes = 1 << 20
	maxReadBytes     = 8 << 20
	maxSearchFile   = 2 << 20
)

type Resolver interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Service struct{ resolver Resolver }

func New(resolver Resolver) (*Service, error) {
	if resolver == nil {
		return nil, errors.New("workspaceflow: resolver is required")
	}
	return &Service{resolver: resolver}, nil
}

func (service *Service) ReadFile(ctx context.Context, request protocol.ReadFileRequest) (*protocol.FileContent, error) {
	_, path, relative, err := service.file(ctx, request.Workspace.Path, request.Path)
	if err != nil {
		return nil, err
	}
	windowed := request.StartLine != 0 || request.EndLine != 0
	start, end := request.StartLine, request.EndLine
	if start <= 0 {
		start = 1
	}
	if end < 0 || (end != 0 && end < start) {
		return nil, fmt.Errorf("%w: invalid line range", protocol.ErrInvalidParams)
	}
	limit := request.MaxBytes
	if limit <= 0 {
		limit = defaultReadBytes
	}
	if limit > maxReadBytes {
		limit = maxReadBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("workspaceflow: open %s: %w", relative, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxReadBytes+1)
	var content strings.Builder
	total, selected, servedEnd := 0, 0, 0
	clipped := false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		total++
		line := scanner.Bytes()
		if bytes.IndexByte(line, 0) >= 0 || !utf8.Valid(line) {
			return nil, fmt.Errorf("%w: %s is not UTF-8 text", protocol.ErrUnsupportedMime, relative)
		}
		if clipped || total < start || (end != 0 && total > end) {
			continue
		}
		if selected > 0 {
			if content.Len() == limit {
				clipped = true
				continue
			}
			content.WriteByte('\n')
		}
		remaining := limit - content.Len()
		if len(line) > remaining {
			prefix := remaining
			for prefix > 0 && !utf8.Valid(line[:prefix]) {
				prefix--
			}
			content.Write(line[:prefix])
			clipped = true
		} else {
			content.Write(line)
		}
		selected++
		servedEnd = total
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("workspaceflow: scan %s: %w", relative, err)
	}
	if start > total && !(total == 0 && start == 1) {
		return nil, fmt.Errorf("%w: requested line range is outside the file", protocol.ErrInvalidParams)
	}
	if end == 0 || end > total {
		end = total
	}
	result := &protocol.FileContent{
		Path: relative, Content: content.String(), Encoding: "utf-8", TotalLines: total,
		Truncated: clipped || (windowed && (start > 1 || end < total)),
	}
	if windowed {
		result.StartLine = start
		result.EndLine = end
		if clipped {
			result.EndLine = servedEnd
		}
	}
	return result, nil
}

func (service *Service) Head(ctx context.Context, request protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	lines := request.Lines
	if lines <= 0 {
		lines = 40
	}
	if lines > 400 {
		lines = 400
	}
	content, err := service.ReadFile(ctx, protocol.ReadFileRequest{
		Workspace: request.Workspace, Path: request.Path, StartLine: 1, EndLine: lines,
	})
	if err != nil {
		return nil, err
	}
	text := []string{}
	if content.TotalLines > 0 {
		text = strings.Split(content.Content, "\n")
	}
	result := make([]protocol.FileLine, 0, len(text))
	for index, line := range text {
		result = append(result, protocol.FileLine{LineNumber: index + 1, Text: line})
	}
	return &protocol.FileHead{Path: content.Path, Lines: result}, nil
}

func (service *Service) ListFiles(ctx context.Context, request protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
	root, start, _, err := service.directory(ctx, request.Workspace.Path, request.Path)
	if err != nil {
		return nil, err
	}
	entries := make([]protocol.FileEntry, 0)
	walk := request.Recursive || request.Glob != ""
	visit := func(path string, entry os.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !request.IncludeIgnored && ignored(relative, entry.IsDir()) {
			if entry.IsDir() && walk {
				return filepath.SkipDir
			}
			return nil
		}
		if request.Glob != "" {
			matched, matchErr := filepath.Match(request.Glob, relative)
			if matchErr != nil {
				return fmt.Errorf("%w: invalid glob: %v", protocol.ErrInvalidParams, matchErr)
			}
			if !matched {
				return nil
			}
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := protocol.FileEntryFile
		if entry.Type()&os.ModeSymlink != 0 {
			kind = protocol.FileEntrySymlink
		} else if entry.IsDir() {
			kind = protocol.FileEntryDir
		}
		value := protocol.FileEntry{
			Path: relative, Name: entry.Name(), Type: kind,
			ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		}
		if kind == protocol.FileEntryFile {
			size := info.Size()
			value.SizeBytes = &size
		}
		entries = append(entries, value)
		return nil
	}
	if walk {
		err = filepath.WalkDir(start, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			return visit(path, entry)
		})
	} else {
		children, readErr := os.ReadDir(start)
		err = readErr
		for _, child := range children {
			if err := visit(filepath.Join(start, child.Name()), child); err != nil {
				return nil, err
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("workspaceflow: list files: %w", err)
	}
	slices.SortFunc(entries, func(left, right protocol.FileEntry) int { return strings.Compare(left.Path, right.Path) })
	startAt, err := decodePathCursor(request.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", protocol.ErrInvalidParams)
	}
	if startAt != "" {
		index, _ := slices.BinarySearchFunc(entries, startAt, func(entry protocol.FileEntry, path string) int {
			return strings.Compare(entry.Path, path)
		})
		for index < len(entries) && entries[index].Path <= startAt {
			index++
		}
		entries = entries[index:]
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	next := ""
	if len(entries) > limit {
		next = encodePathCursor(entries[limit-1].Path)
		entries = entries[:limit]
	}
	return protocol.NewPageWithCursor(entries, next), nil
}

func (service *Service) Grep(ctx context.Context, request protocol.GrepRequest) (*protocol.GrepResult, error) {
	if request.Query == "" {
		return nil, fmt.Errorf("%w: query is required", protocol.ErrInvalidParams)
	}
	root, start, _, err := service.directory(ctx, request.Workspace.Path, request.Path)
	if err != nil {
		return nil, err
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	result := &protocol.GrepResult{Matches: []protocol.GrepMatch{}}
	err = filepath.WalkDir(start, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			if relative != "." && ignored(filepath.ToSlash(relative), true) {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxSearchFile {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if strings.Contains(text, request.Query) {
				result.Total++
				if len(result.Matches) < limit {
					result.Matches = append(result.Matches, protocol.GrepMatch{
						Path: filepath.ToSlash(relative), LineNumber: line, Text: text,
					})
				}
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		return errors.Join(scanErr, closeErr)
	})
	if err != nil {
		return nil, fmt.Errorf("workspaceflow: search: %w", err)
	}
	return result, nil
}

func (service *Service) Changes(ctx context.Context, workspace protocol.WorkspaceRef) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	repository, err := service.repository(ctx, workspace.Path)
	if err != nil {
		return nil, err
	}
	changes, err := service.workspaceChanges(ctx, repository)
	if err != nil {
		return nil, err
	}
	head, err := hasHead(ctx, repository.root)
	if err != nil {
		return nil, err
	}
	if head {
		stats, statsErr := diffStats(ctx, repository, "HEAD")
		if statsErr != nil {
			return nil, statsErr
		}
		for index := range changes {
			if value, found := stats[changes[index].Path]; found {
				changes[index].Added = value.added
				changes[index].Removed = value.removed
				changes[index].Binary = value.binary
			}
		}
	}
	for index := range changes {
		if head && changes[index].Status != protocol.FileStatusUntracked {
			continue
		}
		if changes[index].Status == protocol.FileStatusDeleted {
			continue
		}
		added, binary, statErr := addedFileStat(ctx, repository, changes[index].Path)
		if statErr != nil {
			return nil, statErr
		}
		changes[index].Binary = binary
		if !binary {
			changes[index].Added = &added
			removed := 0
			changes[index].Removed = &removed
		}
	}
	return protocol.NewPage(changes), nil
}

func (service *Service) Diff(ctx context.Context, request protocol.GetDiffRequest) (*protocol.Diff, error) {
	repository, err := service.repository(ctx, request.Workspace.Path)
	if err != nil {
		return nil, err
	}
	path := ""
	if request.Path != "" {
		relative, jailErr := repository.workspaceRequestPath(request.Path)
		if jailErr != nil {
			return nil, jailErr
		}
		path = relative
	}
	changes, err := service.workspaceChanges(ctx, repository)
	if err != nil {
		return nil, err
	}
	patch, includeAddedFiles, err := trackedPatch(ctx, repository, request.Mode, path)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		if path != "" && change.Path != path {
			continue
		}
		if change.Status == protocol.FileStatusDeleted {
			continue
		}
		if !includeAddedFiles && change.Status != protocol.FileStatusUntracked {
			continue
		}
		addition, patchErr := addedFilePatch(ctx, repository, change.Path)
		if patchErr != nil {
			return nil, patchErr
		}
		patch, err = appendPatch(patch, addition)
		if err != nil {
			return nil, err
		}
	}
	if request.Format == protocol.DiffFormatRaw {
		return &protocol.Diff{Patch: string(patch)}, nil
	}
	files, truncated := parsePatch(string(patch), request.Limit)
	files = projectDiffFiles(
		repository,
		files,
		changes,
		path,
		request.Mode != protocol.DiffModeBase,
	)
	return &protocol.Diff{Files: files, Truncated: truncated}, nil
}

func (service *Service) file(ctx context.Context, workspacePath, requested string) (string, string, string, error) {
	root, path, relative, err := service.fileOrMissing(ctx, workspacePath, requested)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", fmt.Errorf("workspaceflow: inspect %s: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", "", fmt.Errorf("workspaceflow: %s is not a regular file", relative)
	}
	return root, path, relative, nil
}

func (service *Service) directory(ctx context.Context, workspacePath, requested string) (string, string, string, error) {
	root, path, relative, err := service.fileOrMissing(ctx, workspacePath, requested)
	if err != nil {
		return "", "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", "", fmt.Errorf("workspaceflow: inspect %s: %w", relative, err)
	}
	if !info.IsDir() {
		return "", "", "", fmt.Errorf("workspaceflow: %s is not a directory", relative)
	}
	return root, path, relative, nil
}

func (service *Service) fileOrMissing(ctx context.Context, workspacePath, requested string) (string, string, string, error) {
	resolved, err := service.resolver.Resolve(ctx, workspacePath)
	if err != nil || !resolved.Available {
		return "", "", "", fmt.Errorf("%w: workspace is unavailable", protocol.ErrWorkspaceUnavailable)
	}
	root := resolved.Workspace.Path()
	if filepath.IsAbs(requested) {
		return "", "", "", fmt.Errorf("%w: path must be relative", protocol.ErrPathOutsideRoot)
	}
	candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(requested)))
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", protocol.ErrPathOutsideRoot
	}
	if real, evalErr := filepath.EvalSymlinks(candidate); evalErr == nil {
		realRelative, relErr := filepath.Rel(root, real)
		if relErr != nil || realRelative == ".." || strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
			return "", "", "", protocol.ErrPathOutsideRoot
		}
		candidate = real
	}
	return root, candidate, filepath.ToSlash(relative), nil
}

func ignored(path string, directory bool) bool {
	for _, part := range strings.Split(path, "/") {
		switch part {
		case ".git", "node_modules", "dist", "build", ".next", ".cache":
			return true
		}
	}
	return directory && strings.HasPrefix(filepath.Base(path), ".")
}

func encodePathCursor(path string) string { return base64.RawURLEncoding.EncodeToString([]byte(path)) }

func decodePathCursor(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return string(decoded), err
}
