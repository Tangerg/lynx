package codebaseflow

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app2/runtime/domain/codebase"
)

const (
	maxIndexFiles       = 5_000
	maxIndexBytes int64 = 64 << 20
	maxIndexChunks      = 20_000
	maxSourceFileBytes  = 512 << 10
	maxSourcePathBytes  = 16 << 10
	maxSourceListBytes  = 8 << 20
	maxChunkLines       = 80
	chunkOverlapLines   = 16
	maxChunkBytes       = 32 << 10
)

var errSourceLimit = errors.New("codebaseflow: source file limit reached")

var sourceExtensions = map[string]struct{}{
	".adoc": {}, ".astro": {}, ".bat": {}, ".c": {}, ".cc": {},
	".clj": {}, ".cmd": {}, ".cpp": {}, ".cs": {}, ".css": {},
	".dart": {}, ".ex": {}, ".exs": {}, ".fish": {}, ".go": {},
	".gql": {}, ".gradle": {}, ".graphql": {}, ".groovy": {}, ".h": {},
	".hpp": {}, ".hs": {}, ".htm": {}, ".html": {}, ".ini": {},
	".java": {}, ".js": {}, ".json": {}, ".jsonc": {}, ".jsx": {},
	".kt": {}, ".lua": {}, ".m": {}, ".md": {}, ".mm": {}, ".nix": {},
	".php": {}, ".pl": {}, ".pm": {}, ".proto": {}, ".ps1": {},
	".py": {}, ".r": {}, ".rb": {}, ".rs": {}, ".rst": {}, ".scala": {},
	".scss": {}, ".sh": {}, ".sol": {}, ".sql": {}, ".svelte": {},
	".swift": {}, ".tf": {}, ".tfvars": {}, ".toml": {}, ".ts": {},
	".tsx": {}, ".txt": {}, ".vue": {}, ".xml": {}, ".yaml": {},
	".yml": {}, ".zig": {},
}

var sourceBasenames = map[string]struct{}{
	"build": {}, "cmakelists.txt": {}, "dockerfile": {}, "gemfile": {},
	"justfile": {}, "makefile": {}, "rakefile": {}, "workspace": {},
}

var skippedSourceFiles = map[string]struct{}{
	"cargo.lock": {}, "go.sum": {}, "package-lock.json": {},
	"pnpm-lock.yaml": {}, "yarn.lock": {},
}

var skippedDirectories = map[string]struct{}{
	".cache": {}, ".git": {}, ".idea": {}, ".next": {}, ".venv": {},
	".vscode": {}, "__pycache__": {}, "bin": {}, "build": {}, "dist": {},
	"node_modules": {}, "obj": {}, "out": {}, "target": {}, "vendor": {},
}

type scannedCorpus struct {
	documents []codebase.Document
	fileCount int
	truncated bool
}

func scanWorkspace(ctx context.Context, root string) (scannedCorpus, error) {
	paths, truncated, err := discoverSourceFiles(ctx, root)
	if err != nil {
		return scannedCorpus{}, err
	}
	corpus := scannedCorpus{
		documents: make([]codebase.Document, 0),
		truncated: truncated,
	}
	var consumed int64
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return scannedCorpus{}, err
		}
		absolute, size, ok := sourceFile(root, path)
		if !ok {
			continue
		}
		if consumed+size > maxIndexBytes {
			corpus.truncated = true
			break
		}
		documents, bytesRead, ok, err := readSourceDocuments(ctx, absolute, path)
		if err != nil {
			return scannedCorpus{}, err
		}
		if consumed+int64(bytesRead) > maxIndexBytes {
			corpus.truncated = true
			break
		}
		consumed += int64(bytesRead)
		if !ok {
			continue
		}
		if len(corpus.documents)+len(documents) > maxIndexChunks {
			corpus.truncated = true
			break
		}
		corpus.documents = append(corpus.documents, documents...)
		corpus.fileCount++
	}
	return corpus, nil
}

func discoverSourceFiles(
	ctx context.Context,
	root string,
) ([]string, bool, error) {
	paths, truncated, repository, err := gitSourceFiles(ctx, root)
	if err != nil {
		return nil, false, err
	}
	if !repository {
		paths, truncated, err = walkedSourceFiles(ctx, root)
		if err != nil {
			return nil, false, err
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	return paths, truncated, nil
}

func gitSourceFiles(
	ctx context.Context,
	root string,
) ([]string, bool, bool, error) {
	command := exec.CommandContext(
		ctx,
		"git", "-C", root, "--no-pager", "ls-files",
		"--cached", "--others", "--exclude-standard", "-z", "--", ".",
	)
	command.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, false, false, fmt.Errorf("codebaseflow: open git source list: %w", err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, false, ctxErr
		}
		return nil, false, false, nil
	}

	paths := make([]string, 0, min(maxIndexFiles, 512))
	listedBytes := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanNUL)
	scanner.Buffer(make([]byte, 4<<10), maxSourcePathBytes)
	truncated := false
	for scanner.Scan() {
		path, ok := sourcePath(scanner.Text())
		if !ok {
			continue
		}
		if len(paths) == maxIndexFiles ||
			listedBytes+len(path) > maxSourceListBytes {
			truncated = true
			break
		}
		paths = append(paths, path)
		listedBytes += len(path)
	}
	if truncated || scanner.Err() != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, false, false, ctxErr
	}
	if scanErr := scanner.Err(); scanErr != nil && !truncated {
		return nil, false, false, fmt.Errorf("codebaseflow: scan git source list: %w", scanErr)
	}
	if waitErr != nil && !truncated {
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			return nil, false, false, nil
		}
		return nil, false, false, fmt.Errorf("codebaseflow: wait for git source list: %w", waitErr)
	}
	return paths, truncated, true, nil
}

func walkedSourceFiles(
	ctx context.Context,
	root string,
) ([]string, bool, error) {
	paths := make([]string, 0, min(maxIndexFiles, 512))
	listedBytes := 0
	truncated := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil && path == root {
			return walkErr
		}
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root {
				if _, skip := skippedDirectories[entry.Name()]; skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		relative, ok := sourcePath(filepath.ToSlash(relative))
		if !ok {
			return nil
		}
		if len(paths) == maxIndexFiles ||
			listedBytes+len(relative) > maxSourceListBytes {
			truncated = true
			return errSourceLimit
		}
		paths = append(paths, relative)
		listedBytes += len(relative)
		return nil
	})
	if errors.Is(err, errSourceLimit) {
		err = nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("codebaseflow: walk source files: %w", err)
	}
	return paths, truncated, nil
}

func sourcePath(path string) (string, bool) {
	if path == "" || !utf8.ValidString(path) || filepath.IsAbs(path) {
		return "", false
	}
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return "", false
	}
	base := strings.ToLower(filepath.Base(path))
	if _, skip := skippedSourceFiles[base]; skip {
		return "", false
	}
	_, namedSource := sourceBasenames[base]
	_, knownExtension := sourceExtensions[strings.ToLower(filepath.Ext(base))]
	indexable := namedSource || knownExtension
	return path, indexable
}

func sourceFile(root, relative string) (string, int64, bool) {
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(absolute)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSourceFileBytes {
		return "", 0, false
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", 0, false
	}
	realRelative, err := filepath.Rel(root, real)
	if err != nil || realRelative == ".." ||
		strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
		return "", 0, false
	}
	return real, info.Size(), true
}

func readSourceDocuments(
	ctx context.Context,
	absolute string,
	relative string,
) ([]codebase.Document, int, bool, error) {
	file, err := os.Open(absolute)
	if err != nil {
		return nil, 0, false, nil
	}
	data, readErr := readSourceFile(ctx, file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, len(data), false, readErr
	}
	if closeErr != nil {
		return nil, len(data), false, fmt.Errorf(
			"codebaseflow: close %s: %w",
			relative,
			closeErr,
		)
	}
	if len(data) > maxSourceFileBytes || bytes.IndexByte(data, 0) >= 0 ||
		!utf8.Valid(data) {
		return nil, len(data), false, nil
	}
	documents, ok := chunkSource(relative, strings.Split(string(data), "\n"))
	return documents, len(data), ok, nil
}

func readSourceFile(ctx context.Context, file *os.File) ([]byte, error) {
	buffer := make([]byte, 64<<10)
	content := bytes.NewBuffer(make([]byte, 0, maxSourceFileBytes))
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, err := file.Read(buffer)
		if count > 0 {
			remaining := maxSourceFileBytes + 1 - content.Len()
			content.Write(buffer[:min(count, remaining)])
			if content.Len() > maxSourceFileBytes {
				return content.Bytes(), nil
			}
		}
		if errors.Is(err, io.EOF) {
			return content.Bytes(), nil
		}
		if err != nil {
			return nil, fmt.Errorf("codebaseflow: read source file: %w", err)
		}
	}
}

func chunkSource(path string, lines []string) ([]codebase.Document, bool) {
	documents := make([]codebase.Document, 0)
	for start := 0; start < len(lines); {
		end := start
		length := 0
		for end < len(lines) && end-start < maxChunkLines {
			lineBytes := len(lines[end])
			if lineBytes > maxChunkBytes {
				return nil, false
			}
			separator := 0
			if end > start {
				separator = 1
			}
			if length+separator+lineBytes > maxChunkBytes {
				break
			}
			length += separator + lineBytes
			end++
		}
		if end == start {
			return nil, false
		}
		snippet := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(snippet) != "" {
			documents = append(documents, codebase.Document{
				Path: path, StartLine: start + 1, EndLine: end, Snippet: snippet,
			})
		}
		if end == len(lines) {
			break
		}
		next := end - min(chunkOverlapLines, end-start-1)
		if next <= start {
			next = end
		}
		start = next
	}
	return documents, len(documents) > 0
}

func scanNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexByte(data, 0); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
