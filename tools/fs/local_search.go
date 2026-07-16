package fs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Default result caps applied when the caller leaves MaxResults at 0.
// The defaults prevent LLM context bloat without forcing the LLM to pass a
// cap on every call.
const (
	defaultGrepMaxResults = 250
	defaultGlobMaxResults = 100
)

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
	for i, path := range paths {
		if relative, err := filepath.Rel(root, path); err == nil {
			paths[i] = relative
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
		if path, err := exec.LookPath("rg"); err == nil {
			l.rgPath = path
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
	default:
		args = append(args, "-n")
		before, after := in.contextLines()
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
	args := []string{"-rE"}

	switch mode {
	case GrepOutputFilesWithMatches:
		args = append(args, "-l")
	case GrepOutputCount:
		args = append(args, "-c")
	default:
		args = append(args, "-n")
		before, after := in.contextLines()
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
	default:
		matches := parseGrepLines(out)
		truncated := false
		if len(matches) > maxResults {
			matches = matches[:maxResults]
			truncated = true
		}
		return GrepOutput{Matches: matches, Truncated: truncated}
	}
}

// parseGrepCounts handles the `path:count` output of `grep -c` / `rg -c`.
// Split on the last colon because file paths may contain colons.
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
