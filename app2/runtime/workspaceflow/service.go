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
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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
	root, err := service.repository(ctx, workspace.Path)
	if err != nil {
		return nil, err
	}
	encoded, err := git(ctx, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	changes, err := parseStatus(encoded)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(changes), nil
}

func (service *Service) Diff(ctx context.Context, request protocol.GetDiffRequest) (*protocol.Diff, error) {
	root, err := service.repository(ctx, request.Workspace.Path)
	if err != nil {
		return nil, err
	}
	arguments := []string{"diff", "--no-ext-diff", "--no-color"}
	if request.Mode == protocol.DiffModeBase {
		base, baseErr := mergeBase(ctx, root)
		if baseErr != nil {
			return nil, baseErr
		}
		arguments = append(arguments, base)
	}
	if request.Path != "" {
		_, _, relative, jailErr := service.fileOrMissing(ctx, request.Workspace.Path, request.Path)
		if jailErr != nil {
			return nil, jailErr
		}
		arguments = append(arguments, "--", relative)
	}
	patch, err := git(ctx, root, arguments...)
	if err != nil {
		return nil, err
	}
	if request.Format == protocol.DiffFormatRaw {
		return &protocol.Diff{Patch: string(patch)}, nil
	}
	files, truncated := parsePatch(string(patch), request.Limit)
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

func (service *Service) repository(ctx context.Context, workspacePath string) (string, error) {
	resolved, err := service.resolver.Resolve(ctx, workspacePath)
	if err != nil || !resolved.Available {
		return "", fmt.Errorf("%w: workspace is unavailable", protocol.ErrWorkspaceUnavailable)
	}
	if _, err := os.Lstat(filepath.Join(resolved.ProjectRoot, ".git")); err != nil {
		return "", protocol.ErrVcsUnavailable
	}
	return resolved.ProjectRoot, nil
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

func git(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root, "--no-pager"}, arguments...)...)
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, fmt.Errorf("workspaceflow: git %s: %s", arguments[0], strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("workspaceflow: git %s: %w", arguments[0], err)
	}
	return output, nil
}

func parseStatus(encoded []byte) ([]protocol.WorkspaceFileChange, error) {
	fields := bytes.Split(encoded, []byte{0})
	changes := make([]protocol.WorkspaceFileChange, 0, len(fields))
	for index := 0; index < len(fields); index++ {
		line := string(fields[index])
		if line == "" {
			continue
		}
		if len(line) < 4 {
			return nil, errors.New("workspaceflow: malformed git status")
		}
		code, path := line[:2], filepath.ToSlash(line[3:])
		change := protocol.WorkspaceFileChange{Path: path, Status: statusOf(code), Added: new(int), Removed: new(int)}
		if strings.Contains(code, "R") && index+1 < len(fields) {
			index++
			change.PreviousPath = filepath.ToSlash(string(fields[index]))
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func statusOf(code string) protocol.FileStatus {
	switch {
	case code == "??":
		return protocol.FileStatusUntracked
	case strings.Contains(code, "R"):
		return protocol.FileStatusRenamed
	case strings.Contains(code, "D"):
		return protocol.FileStatusDeleted
	case strings.Contains(code, "A"):
		return protocol.FileStatusAdded
	default:
		return protocol.FileStatusModified
	}
}

func mergeBase(ctx context.Context, root string) (string, error) {
	for _, ref := range []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"} {
		output, err := git(ctx, root, "merge-base", "HEAD", ref)
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.TrimSpace(string(output)), nil
		}
	}
	return "", protocol.ErrVcsUnavailable
}

func parsePatch(patch string, limit int) ([]protocol.FileDiff, bool) {
	if limit <= 0 {
		limit = 5000
	}
	files := make([]protocol.FileDiff, 0)
	var current *protocol.FileDiff
	left, right, rows := 0, 0, 0
	flush := func() {
		if current != nil {
			files = append(files, *current)
		}
	}
	truncated := false
	for scanner := bufio.NewScanner(strings.NewReader(patch)); scanner.Scan(); {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			parts := strings.SplitN(line, " b/", 2)
			path := ""
			if len(parts) == 2 {
				path = parts[1]
			}
			current = &protocol.FileDiff{Path: path, Status: protocol.FileStatusModified, Rows: []protocol.DiffRow{}}
			left, right = 0, 0
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "new file"):
			current.Status = protocol.FileStatusAdded
		case strings.HasPrefix(line, "deleted file"):
			current.Status = protocol.FileStatusDeleted
		case strings.HasPrefix(line, "rename from "):
			current.Status = protocol.FileStatusRenamed
			current.PreviousPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "Binary files"):
			current.Binary = true
		case strings.HasPrefix(line, "@@"):
			left, right = parseHunk(line)
			current.Rows = append(current.Rows, protocol.DiffRow{Type: protocol.DiffRowHunk, Text: line})
			rows++
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.Rows = append(current.Rows, protocol.DiffRow{Type: protocol.DiffRowAdded, RightLine: right, Code: line[1:]})
			right++
			rows++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.Rows = append(current.Rows, protocol.DiffRow{Type: protocol.DiffRowDeleted, LeftLine: left, Code: line[1:]})
			left++
			rows++
		case strings.HasPrefix(line, " "):
			current.Rows = append(current.Rows, protocol.DiffRow{Type: protocol.DiffRowContext, LeftLine: left, RightLine: right, Code: line[1:]})
			left++
			right++
			rows++
		}
		if rows >= limit {
			truncated = true
			break
		}
	}
	flush()
	for index := range files {
		added, removed := 0, 0
		for _, row := range files[index].Rows {
			if row.Type == protocol.DiffRowAdded {
				added++
			}
			if row.Type == protocol.DiffRowDeleted {
				removed++
			}
		}
		if !files[index].Binary {
			files[index].Added, files[index].Removed = &added, &removed
		}
	}
	return files, truncated
}

func parseHunk(line string) (int, int) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return 0, 0
	}
	parse := func(value string) int {
		value = strings.TrimLeft(value, "+-")
		value, _, _ = strings.Cut(value, ",")
		parsed, _ := strconv.Atoi(value)
		return parsed
	}
	return parse(parts[1]), parse(parts[2])
}

func encodePathCursor(path string) string { return base64.RawURLEncoding.EncodeToString([]byte(path)) }

func decodePathCursor(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return string(decoded), err
}
