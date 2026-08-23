package workspaceflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const (
	maxGitOutput = 64 << 20
	maxGitError  = 64 << 10
)

type repositoryScope struct {
	root      string
	workspace string
	prefix    string
}

func (scope repositoryScope) repositoryPath(relative string) string {
	if relative == "" {
		return scope.prefix
	}
	if scope.prefix == "." {
		return relative
	}
	return filepath.ToSlash(filepath.Join(scope.prefix, filepath.FromSlash(relative)))
}

func (scope repositoryScope) workspaceRequestPath(requested string) (string, error) {
	if filepath.IsAbs(requested) {
		return "", fmt.Errorf("%w: path must be relative", protocol.ErrPathOutsideRoot)
	}
	relative := filepath.Clean(filepath.FromSlash(requested))
	if relative == "." {
		return "", nil
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", protocol.ErrPathOutsideRoot
	}
	return filepath.ToSlash(relative), nil
}

func (scope repositoryScope) workspacePath(repositoryPath string) (string, bool) {
	if repositoryPath == "" {
		return "", false
	}
	relative, err := filepath.Rel(
		filepath.FromSlash(scope.prefix),
		filepath.FromSlash(repositoryPath),
	)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

func (service *Service) repository(ctx context.Context, workspacePath string) (repositoryScope, error) {
	resolved, err := service.resolver.Resolve(ctx, workspacePath)
	if err != nil || !resolved.Available {
		return repositoryScope{}, fmt.Errorf("%w: workspace is unavailable", protocol.ErrWorkspaceUnavailable)
	}
	if _, err := os.Lstat(filepath.Join(resolved.ProjectRoot, ".git")); err != nil {
		return repositoryScope{}, protocol.ErrVcsUnavailable
	}
	prefix, err := filepath.Rel(resolved.ProjectRoot, resolved.Workspace.Path())
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return repositoryScope{}, protocol.ErrPathOutsideRoot
	}
	if prefix == "" {
		prefix = "."
	}
	return repositoryScope{
		root:      resolved.ProjectRoot,
		workspace: resolved.Workspace.Path(),
		prefix:    filepath.ToSlash(prefix),
	}, nil
}

func (service *Service) workspaceChanges(
	ctx context.Context,
	repository repositoryScope,
) ([]protocol.WorkspaceFileChange, error) {
	encoded, err := git(
		ctx,
		repository.root,
		"status", "--porcelain=v1", "-z", "--untracked-files=all", "--", repository.prefix,
	)
	if err != nil {
		return nil, err
	}
	changes, err := parseStatus(encoded)
	if err != nil {
		return nil, err
	}
	projected := make([]protocol.WorkspaceFileChange, 0, len(changes))
	for _, change := range changes {
		path, inside := repository.workspacePath(change.Path)
		previous, previousInside := repository.workspacePath(change.PreviousPath)
		switch {
		case inside && (change.PreviousPath == "" || previousInside):
			change.Path = path
			change.PreviousPath = previous
		case inside:
			change.Path = path
			change.Status = protocol.FileStatusAdded
			change.PreviousPath = ""
		case previousInside:
			change.Path = previous
			change.Status = protocol.FileStatusDeleted
			change.PreviousPath = ""
		default:
			continue
		}
		projected = append(projected, change)
	}
	return projected, nil
}

type fileStats struct {
	added   *int
	removed *int
	binary  bool
}

func diffStats(
	ctx context.Context,
	repository repositoryScope,
	base string,
) (map[string]fileStats, error) {
	encoded, err := git(
		ctx,
		repository.root,
		"diff", "--numstat", "--no-textconv", "-M", "-z", base, "--", repository.prefix,
	)
	if err != nil {
		return nil, err
	}
	stats, err := parseNumstat(encoded)
	if err != nil {
		return nil, err
	}
	projected := make(map[string]fileStats, len(stats))
	for path, value := range stats {
		if relative, inside := repository.workspacePath(path); inside {
			projected[relative] = value
		}
	}
	return projected, nil
}

func parseNumstat(encoded []byte) (map[string]fileStats, error) {
	fields := bytes.Split(encoded, []byte{0})
	result := make(map[string]fileStats, len(fields))
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if len(field) == 0 {
			continue
		}
		parts := bytes.SplitN(field, []byte{'\t'}, 3)
		if len(parts) != 3 {
			return nil, errors.New("workspaceflow: malformed git numstat")
		}
		path := string(parts[2])
		if path == "" {
			if index+2 >= len(fields) {
				return nil, errors.New("workspaceflow: malformed git rename numstat")
			}
			index += 2
			path = string(fields[index])
		}
		value := fileStats{binary: string(parts[0]) == "-" || string(parts[1]) == "-"}
		if !value.binary {
			added, addedErr := strconv.Atoi(string(parts[0]))
			removed, removedErr := strconv.Atoi(string(parts[1]))
			if addedErr != nil || removedErr != nil {
				return nil, errors.New("workspaceflow: malformed git numstat count")
			}
			value.added, value.removed = &added, &removed
		}
		result[filepath.ToSlash(path)] = value
	}
	return result, nil
}

func trackedPatch(
	ctx context.Context,
	repository repositoryScope,
	mode protocol.DiffMode,
	path string,
) ([]byte, bool, error) {
	head, err := hasHead(ctx, repository.root)
	if err != nil {
		return nil, false, err
	}
	if !head {
		if mode == protocol.DiffModeBase {
			return nil, false, protocol.ErrVcsUnavailable
		}
		return nil, true, nil
	}
	base := "HEAD"
	if mode == protocol.DiffModeBase {
		base, err = mergeBase(ctx, repository.root)
		if err != nil {
			return nil, false, err
		}
	}
	patch, err := git(
		ctx,
		repository.root,
		"diff", "--no-ext-diff", "--no-textconv", "--no-color", "-M",
		base, "--", repository.repositoryPath(path),
	)
	return patch, false, err
}

func hasHead(ctx context.Context, root string) (bool, error) {
	result, err := runGitCommand(
		ctx,
		root,
		"rev-parse", "--verify", "--quiet", "HEAD",
	)
	if err != nil {
		return false, err
	}
	if result.exitCode == 0 {
		return true, nil
	}
	if result.exitCode == 1 {
		return false, nil
	}
	return false, gitFailure("inspect HEAD", result)
}

func addedFileStat(
	ctx context.Context,
	repository repositoryScope,
	path string,
) (int, bool, error) {
	absolute := filepath.Join(repository.workspace, filepath.FromSlash(path))
	info, err := os.Lstat(absolute)
	if err != nil {
		return 0, false, fmt.Errorf("workspaceflow: inspect added file %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 1, false, nil
	}
	if !info.Mode().IsRegular() {
		return 0, true, nil
	}
	file, err := os.Open(absolute)
	if err != nil {
		return 0, false, fmt.Errorf("workspaceflow: open added file %s: %w", path, err)
	}
	defer file.Close()
	buffer := make([]byte, 64<<10)
	lines := 0
	var last byte
	nonempty := false
	for {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			nonempty = true
			chunk := buffer[:count]
			if bytes.IndexByte(chunk, 0) >= 0 {
				return 0, true, nil
			}
			lines += bytes.Count(chunk, []byte{'\n'})
			last = chunk[len(chunk)-1]
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return 0, false, fmt.Errorf("workspaceflow: read added file %s: %w", path, readErr)
		}
	}
	if nonempty && last != '\n' {
		lines++
	}
	return lines, false, nil
}

func addedFilePatch(ctx context.Context, repository repositoryScope, path string) ([]byte, error) {
	info, err := os.Lstat(filepath.Join(repository.workspace, filepath.FromSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("workspaceflow: inspect added file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return nil, fmt.Errorf(
			"%w: %s is not a regular file or symbolic link",
			protocol.ErrUnsupportedMime,
			path,
		)
	}
	repositoryPath := repository.repositoryPath(path)
	result, err := runGitCommand(
		ctx,
		repository.root,
		"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color",
		"--", os.DevNull, repositoryPath,
	)
	if err != nil {
		return nil, err
	}
	if result.exitCode == 0 || result.exitCode == 1 {
		if len(result.output) == 0 {
			return []byte(fmt.Sprintf(
				"diff --git a/%s b/%s\nnew file mode %o\n--- /dev/null\n+++ b/%s\n",
				repositoryPath, repositoryPath, infoMode(repository.workspace, path), repositoryPath,
			)), nil
		}
		return result.output, nil
	}
	return nil, gitFailure("diff added file "+path, result)
}

func infoMode(workspace, path string) uint32 {
	info, err := os.Lstat(filepath.Join(workspace, filepath.FromSlash(path)))
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return 120000
	}
	if err == nil && info.Mode()&0o111 != 0 {
		return 100755
	}
	return 100644
}

func appendPatch(existing, addition []byte) ([]byte, error) {
	if len(addition) == 0 {
		return existing, nil
	}
	separator := 0
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = 1
	}
	if len(existing)+separator+len(addition) > maxGitOutput {
		return nil, fmt.Errorf(
			"workspaceflow: aggregate git diff exceeds %d bytes",
			maxGitOutput,
		)
	}
	if separator != 0 {
		existing = append(existing, '\n')
	}
	return append(existing, addition...), nil
}

func projectDiffFiles(
	repository repositoryScope,
	files []protocol.FileDiff,
	changes []protocol.WorkspaceFileChange,
	requestedPath string,
	overlayWorkingTree bool,
) []protocol.FileDiff {
	changeByPath := make(map[string]protocol.WorkspaceFileChange, len(changes))
	for _, change := range changes {
		changeByPath[change.Path] = change
	}
	projected := make([]protocol.FileDiff, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		path, inside := repository.workspacePath(file.Path)
		previous, previousInside := repository.workspacePath(file.PreviousPath)
		switch {
		case inside && (file.PreviousPath == "" || previousInside):
			file.Path = path
			file.PreviousPath = previous
		case inside:
			file.Path = path
			file.Status = protocol.FileStatusAdded
			file.PreviousPath = ""
		case previousInside:
			file.Path = previous
			file.Status = protocol.FileStatusDeleted
			file.PreviousPath = ""
		default:
			continue
		}
		if requestedPath != "" && file.Path != requestedPath {
			continue
		}
		if change, found := changeByPath[file.Path]; overlayWorkingTree && found {
			file.Status = change.Status
			file.PreviousPath = change.PreviousPath
		}
		if seen[file.Path] {
			continue
		}
		seen[file.Path] = true
		projected = append(projected, file)
	}
	return projected
}

func git(ctx context.Context, root string, arguments ...string) ([]byte, error) {
	result, err := runGitCommand(ctx, root, arguments...)
	if err != nil {
		return nil, err
	}
	if result.exitCode != 0 {
		return nil, gitFailure(arguments[0], result)
	}
	return result.output, nil
}

type gitCommandResult struct {
	output   []byte
	stderr   string
	exitCode int
}

type boundedText struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (text *boundedText) Write(value []byte) (int, error) {
	length := len(value)
	remaining := max(text.limit-text.buffer.Len(), 0)
	if len(value) > remaining {
		value = value[:remaining]
		text.truncated = true
	}
	_, _ = text.buffer.Write(value)
	return length, nil
}

func (text *boundedText) String() string {
	if text.truncated {
		return text.buffer.String() + "…"
	}
	return text.buffer.String()
}

func runGitCommand(
	ctx context.Context,
	root string,
	arguments ...string,
) (gitCommandResult, error) {
	command := exec.CommandContext(
		ctx,
		"git",
		append([]string{"-C", root, "--no-pager"}, arguments...)...,
	)
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return gitCommandResult{}, fmt.Errorf("workspaceflow: git %s stdout: %w", arguments[0], err)
	}
	stderr := boundedText{limit: maxGitError}
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return gitCommandResult{}, fmt.Errorf("workspaceflow: start git %s: %w", arguments[0], err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxGitOutput+1))
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return gitCommandResult{}, fmt.Errorf(
			"workspaceflow: read git %s output: %w",
			arguments[0],
			readErr,
		)
	}
	if len(output) > maxGitOutput {
		_ = command.Process.Kill()
		_ = command.Wait()
		return gitCommandResult{}, fmt.Errorf(
			"workspaceflow: git %s output exceeds %d bytes",
			arguments[0],
			maxGitOutput,
		)
	}
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return gitCommandResult{}, ctxErr
	}
	result := gitCommandResult{output: output, stderr: stderr.String()}
	if waitErr == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) {
		result.exitCode = exit.ExitCode()
		return result, nil
	}
	return gitCommandResult{}, fmt.Errorf(
		"workspaceflow: wait for git %s: %w",
		arguments[0],
		waitErr,
	)
}

func gitFailure(operation string, result gitCommandResult) error {
	detail := strings.TrimSpace(result.stderr)
	if detail == "" {
		detail = fmt.Sprintf("exit code %d", result.exitCode)
	}
	return fmt.Errorf("workspaceflow: git %s: %s", operation, detail)
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
		change := protocol.WorkspaceFileChange{Path: path, Status: statusOf(code)}
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
	refs := []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"}
	for _, ref := range refs {
		output, err := git(ctx, root, "merge-base", "HEAD", ref)
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.TrimSpace(string(output)), nil
		}
	}
	return "", protocol.ErrVcsUnavailable
}
