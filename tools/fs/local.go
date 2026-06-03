package fs

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Default result caps applied when the caller leaves MaxResults at 0.
// Mirrors Claude Code's defaults (250 grep matches, 100 glob paths) —
// these prevent LLM context bloat without forcing the LLM to pass a
// cap on every call.
const (
	defaultGrepMaxResults = 250
	defaultGlobMaxResults = 100
)

// LocalExecutor is the reference [Executor] running against the host
// filesystem.
//
//   - Glob shells out to find (portable across BSD and GNU; bash 3.2
//     on macOS doesn't honor globstar, so a bash-based impl wouldn't
//     work everywhere).
//   - Grep prefers ripgrep when it's on PATH; falls back to GNU grep
//     otherwise (FileType / Multiline only work on the ripgrep path).
//   - Write and Edit serialize per file via [LocalExecutor.lockPath]
//     so concurrent tool calls on the same path can't tear.
//   - Read normalises CRLF→LF and strips UTF-8 BOM; Write and Edit
//     restore both when the existing file uses them.
type LocalExecutor struct {
	// Root, if set, anchors relative paths. "" = no confinement;
	// the executor trusts the caller. TODO(security): path-jail.
	Root string

	pathLocksMu sync.Mutex
	pathLocks   map[string]*sync.Mutex

	rgOnce sync.Once
	rgPath string // "" after rgOnce runs means rg is not on PATH
}

// NewLocalExecutor returns a [LocalExecutor] anchored at root. Pass
// "" for an unrestricted executor (typical for trusted local dev).
func NewLocalExecutor(root string) *LocalExecutor {
	return &LocalExecutor{Root: root}
}

// resolve combines the executor's Root with a relative path. Absolute
// paths pass through. Empty path is rejected.
func (l *LocalExecutor) resolve(p string) (string, error) {
	if p == "" {
		return "", ErrEmptyPath
	}
	if l.Root == "" || filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(l.Root, p), nil
}

// rootDir returns the directory bulk queries (Glob/Grep) should start
// from. Precedence: caller-supplied Root → executor's Root → CWD.
func (l *LocalExecutor) rootDir(callerRoot string) string {
	return cmp.Or(callerRoot, l.Root, ".")
}

// lockPath returns a per-path mutex unlock func. Used to serialize
// Write and Edit on the same file. The map of locks grows monotonically
// — bounded by the set of paths the agent touches — which is acceptable
// for typical workspace sizes (a few thousand entries × 16 bytes).
func (l *LocalExecutor) lockPath(path string) func() {
	l.pathLocksMu.Lock()
	if l.pathLocks == nil {
		l.pathLocks = map[string]*sync.Mutex{}
	}
	m, ok := l.pathLocks[path]
	if !ok {
		m = &sync.Mutex{}
		l.pathLocks[path] = m
	}
	l.pathLocksMu.Unlock()
	m.Lock()
	return m.Unlock
}

// ---------------------------------------------------------------- Read

// Read does not lock — concurrent reads are fine and a slightly stale
// read while another goroutine writes is acceptable (atomic-rename in
// Write means we either see the old file in full or the new file in
// full, never a torn write).
func (l *LocalExecutor) Read(_ context.Context, in ReadInput) (ReadOutput, error) {
	path, err := l.resolve(in.Path)
	if err != nil {
		return ReadOutput{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReadOutput{}, err
	}
	if looksBinary(data) {
		return ReadOutput{}, ErrBinaryFile
	}

	content, _, _ := normalizeText(data)
	lines := strings.Split(content, "\n")
	total := len(lines)

	start := min(max(in.Offset, 0), total)
	end := total
	if in.Limit > 0 {
		end = min(start+in.Limit, total)
	}

	return ReadOutput{
		Content:    strings.Join(lines[start:end], "\n"),
		StartLine:  start,
		EndLine:    end,
		TotalLines: total,
		Truncated:  end < total,
	}, nil
}

// ---------------------------------------------------------------- Write

func (l *LocalExecutor) Write(_ context.Context, in WriteInput) (WriteOutput, error) {
	if strings.ContainsRune(in.Content, 0) {
		return WriteOutput{}, fmt.Errorf("fs.LocalExecutor.Write: %w", ErrBinaryFile)
	}
	path, err := l.resolve(in.Path)
	if err != nil {
		return WriteOutput{}, err
	}

	unlock := l.lockPath(path)
	defer unlock()

	// Detect existing format + permissions so an overwrite preserves
	// CRLF / BOM / mode instead of silently flipping them.
	mode := os.FileMode(0o644)
	hadBOM, hadCRLF := false, false
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if !in.Append {
			if existing, err := os.ReadFile(path); err == nil {
				hadBOM = hasUTF8BOM(existing)
				hadCRLF = bytes.Contains(existing, []byte("\r\n"))
			}
		}
	}

	if in.Append {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return WriteOutput{}, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, mode)
		if err != nil {
			return WriteOutput{}, err
		}
		defer f.Close()
		n, err := f.WriteString(in.Content)
		if err != nil {
			return WriteOutput{}, err
		}
		return WriteOutput{BytesWritten: n}, nil
	}

	out := restoreFormat(in.Content, hadBOM, hadCRLF)
	if err := atomicWriteFile(path, out, mode); err != nil {
		return WriteOutput{}, err
	}
	return WriteOutput{BytesWritten: len(out)}, nil
}

// ---------------------------------------------------------------- Edit

func (l *LocalExecutor) Edit(_ context.Context, in EditInput) (EditOutput, error) {
	if in.OldString == "" {
		return EditOutput{}, errors.New("fs.LocalExecutor.Edit: old_string must not be empty")
	}
	path, err := l.resolve(in.Path)
	if err != nil {
		return EditOutput{}, err
	}

	unlock := l.lockPath(path)
	defer unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return EditOutput{}, err
	}
	if looksBinary(data) {
		return EditOutput{}, ErrBinaryFile
	}

	content, hadBOM, hadCRLF := normalizeText(data)
	occurrences := strings.Count(content, in.OldString)
	if occurrences == 0 {
		// TODO(fuzzy): retry with normalised quotes / dashes / whitespace
		// before declaring failure (Claude Code's likeMatch trick).
		return EditOutput{}, fmt.Errorf("fs.LocalExecutor.Edit: old_string not found in %s", in.Path)
	}
	if occurrences > 1 && !in.ReplaceAll {
		return EditOutput{}, fmt.Errorf("fs.LocalExecutor.Edit: old_string matches %d times in %s — set replace_all=true to confirm", occurrences, in.Path)
	}

	// strings.Replace with n=-1 is ReplaceAll; n=1 is single-shot.
	n := 1
	if in.ReplaceAll {
		n = -1
	}
	updated := strings.Replace(content, in.OldString, in.NewString, n)

	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	out := restoreFormat(updated, hadBOM, hadCRLF)
	if err := atomicWriteFile(path, out, mode); err != nil {
		return EditOutput{}, err
	}

	replacements := 1
	if in.ReplaceAll {
		replacements = occurrences
	}
	return EditOutput{Replacements: replacements}, nil
}

// ---------------------------------------------------------------- Glob

// Glob shells out to find for portability. We don't use bash's
// `shopt -s globstar` because macOS still ships bash 3.2, which
// doesn't honor it; find is universally available and supports
// -maxdepth on both BSD and GNU variants.
//
// Supported pattern shapes:
//   - "**/*.go"       → recursive from root, leaf "*.go"
//   - "src/**/*.ts"   → recursive from root/src, leaf "*.ts"
//   - "*.go"          → single level
//   - "cmd/main.go"   → single level under cmd/
//
// Unsupported (yet): patterns with ** in the middle ("cmd/*/main.go"),
// multiple **, brace expansion. The LLM-facing tool doc lists the
// supported shapes.
func (l *LocalExecutor) Glob(ctx context.Context, in GlobInput) (GlobOutput, error) {
	if in.Pattern == "" {
		return GlobOutput{}, ErrEmptyPattern
	}
	root := l.rootDir(in.Root)
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultGlobMaxResults
	}

	anchor, args := globPatternToFindArgs(in.Pattern, root, in.IgnoreCase)
	out, err := exec.CommandContext(ctx, "find", append([]string{anchor}, args...)...).Output()
	if err != nil {
		return GlobOutput{}, fmt.Errorf("fs.LocalExecutor.Glob: %w", err)
	}

	paths := splitLines(out)
	// Return paths relative to root so the LLM sees workspace-relative
	// strings, not absolute /private/var/folders/... clutter.
	for i, p := range paths {
		if rel, err := filepath.Rel(root, p); err == nil {
			paths[i] = rel
		}
	}

	truncated := false
	if len(paths) > maxResults {
		paths = paths[:maxResults]
		truncated = true
	}
	return GlobOutput{Paths: paths, Truncated: truncated}, nil
}

// globPatternToFindArgs translates a doublestar glob into find(1)
// arguments. See [LocalExecutor.Glob] for supported shapes.
func globPatternToFindArgs(pattern, root string, ignoreCase bool) (anchor string, args []string) {
	nameFlag := "-name"
	if ignoreCase {
		nameFlag = "-iname"
	}
	args = []string{"-type", "f"}

	if prefix, suffix, found := strings.Cut(pattern, "**/"); found {
		anchor = filepath.Join(root, prefix)
		if suffix != "" {
			args = append(args, nameFlag, suffix)
		}
		return anchor, args
	}
	if pattern == "**" {
		return root, args
	}
	// No ** in pattern → single-level glob.
	dir, leaf := filepath.Split(pattern)
	if dir == "" {
		anchor = root
	} else {
		anchor = filepath.Join(root, dir)
	}
	args = append(args, "-maxdepth", "1", nameFlag, leaf)
	return anchor, args
}

// ---------------------------------------------------------------- Grep

func (l *LocalExecutor) Grep(ctx context.Context, in GrepInput) (GrepOutput, error) {
	if in.Pattern == "" {
		return GrepOutput{}, ErrEmptyPattern
	}
	mode := in.OutputMode
	if mode == "" {
		mode = GrepOutputContent
	}
	if mode != GrepOutputContent && mode != GrepOutputFilesWithMatches && mode != GrepOutputCount {
		return GrepOutput{}, fmt.Errorf("fs.LocalExecutor.Grep: invalid output_mode %q", in.OutputMode)
	}
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultGrepMaxResults
	}
	if l.ripgrep() != "" {
		return l.grepWithRipgrep(ctx, in, mode, maxResults)
	}
	return l.grepWithGNU(ctx, in, mode, maxResults)
}

// ripgrep caches the rg lookup. Empty string means rg is not on PATH.
func (l *LocalExecutor) ripgrep() string {
	l.rgOnce.Do(func() {
		if p, err := exec.LookPath("rg"); err == nil {
			l.rgPath = p
		}
	})
	return l.rgPath
}

func (l *LocalExecutor) grepWithRipgrep(ctx context.Context, in GrepInput, mode GrepOutputMode, maxResults int) (GrepOutput, error) {
	root := l.rootDir(in.Root)
	args := []string{"--no-heading", "--color=never"}

	switch mode {
	case GrepOutputFilesWithMatches:
		args = append(args, "-l")
	case GrepOutputCount:
		args = append(args, "-c")
	default: // content
		args = append(args, "-n")
		before, after := resolveContext(in)
		if before > 0 {
			args = append(args, "-B", strconv.Itoa(before))
		}
		if after > 0 {
			args = append(args, "-A", strconv.Itoa(after))
		}
	}
	if in.IgnoreCase {
		args = append(args, "-i")
	}
	if in.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if in.FileType != "" {
		args = append(args, "-t", in.FileType)
	}
	if in.Glob != "" {
		args = append(args, "-g", in.Glob)
	}
	args = append(args, "-e", in.Pattern, root)

	out, err := runGrep(ctx, l.ripgrep(), args)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("fs.LocalExecutor.Grep(rg): %w", err)
	}
	return shapeGrepOutput(mode, out, maxResults), nil
}

func (l *LocalExecutor) grepWithGNU(ctx context.Context, in GrepInput, mode GrepOutputMode, maxResults int) (GrepOutput, error) {
	root := l.rootDir(in.Root)
	args := []string{"-rE"} // recursive, extended regex

	switch mode {
	case GrepOutputFilesWithMatches:
		args = append(args, "-l")
	case GrepOutputCount:
		args = append(args, "-c")
	default: // content
		args = append(args, "-n")
		before, after := resolveContext(in)
		if before > 0 {
			args = append(args, "-B", strconv.Itoa(before))
		}
		if after > 0 {
			args = append(args, "-A", strconv.Itoa(after))
		}
	}
	if in.IgnoreCase {
		args = append(args, "-i")
	}
	if in.Glob != "" {
		args = append(args, "--include="+in.Glob)
	}
	// FileType / Multiline are silently ignored — GNU grep can't.
	args = append(args, "-e", in.Pattern, root)

	out, err := runGrep(ctx, "grep", args)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("fs.LocalExecutor.Grep: %w", err)
	}
	return shapeGrepOutput(mode, out, maxResults), nil
}

// resolveContext returns (before, after) lines using the explicit
// BeforeContext / AfterContext when set, otherwise the symmetric
// Context fallback.
func resolveContext(in GrepInput) (before, after int) {
	return cmp.Or(in.BeforeContext, in.Context), cmp.Or(in.AfterContext, in.Context)
}

// runGrep runs a grep-family binary and treats exit code 1 (no
// matches) as a clean empty result, not an error.
func runGrep(ctx context.Context, bin string, args []string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err == nil {
		return out, nil
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		return nil, nil
	}
	return out, err
}

func shapeGrepOutput(mode GrepOutputMode, out []byte, maxResults int) GrepOutput {
	switch mode {
	case GrepOutputFilesWithMatches:
		files := splitLines(out)
		truncated := false
		if len(files) > maxResults {
			files = files[:maxResults]
			truncated = true
		}
		return GrepOutput{Files: files, Truncated: truncated}
	case GrepOutputCount:
		counts := parseGrepCounts(out)
		truncated := false
		if len(counts) > maxResults {
			counts = counts[:maxResults]
			truncated = true
		}
		return GrepOutput{Counts: counts, Truncated: truncated}
	default: // content
		matches := parseGrepLines(out)
		truncated := false
		if len(matches) > maxResults {
			matches = matches[:maxResults]
			truncated = true
		}
		return GrepOutput{Matches: matches, Truncated: truncated}
	}
}

// parseGrepCounts handles the `path:count` output of `grep -c` /
// `rg -c`. GNU grep emits one line per searched file (including
// zero-match files); rg only emits matching files. We filter the
// zero-count lines either way. Split on the LAST colon because file
// paths may contain colons on some filesystems.
func parseGrepCounts(out []byte) []GrepFileCount {
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil
	}
	var counts []GrepFileCount
	for line := range strings.SplitSeq(text, "\n") {
		i := strings.LastIndex(line, ":")
		if i <= 0 {
			continue
		}
		n, err := strconv.Atoi(line[i+1:])
		if err != nil || n == 0 {
			continue
		}
		counts = append(counts, GrepFileCount{Path: line[:i], Count: n})
	}
	return counts
}

// ---------------------------------------------------------------- helpers

// binarySniffLen matches git's heuristic — a NUL in the first 8 KiB
// means we treat the file as binary.
const binarySniffLen = 8192

func looksBinary(data []byte) bool {
	sniff := data
	if len(sniff) > binarySniffLen {
		sniff = sniff[:binarySniffLen]
	}
	return bytes.IndexByte(sniff, 0) >= 0
}

func splitLines(out []byte) []string {
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func parseGrepLines(out []byte) []GrepMatch {
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil
	}
	var matches []GrepMatch
	for line := range strings.SplitSeq(text, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		lineNum, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		matches = append(matches, GrepMatch{
			Path: parts[0],
			Line: lineNum,
			Text: parts[2],
		})
	}
	return matches
}
