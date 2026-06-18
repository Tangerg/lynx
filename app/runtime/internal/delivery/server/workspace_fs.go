package server

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/tools/fs"
)

// workspace.* filesystem-backed reads (API.md §7.5): file preview + grep,
// both jailed to the workspace root.

// defaultFileHeadLines caps a workspace.getFileHead preview when the client
// gives no (or a non-positive) line count.
const defaultFileHeadLines = 200

// WorkspaceGetFileHead returns the first N lines of a cwd-relative file
// (API.md §7.5). The path is jailed to the workspace root (resolveInRoot);
// binary files surface fs.ErrBinaryFile. Lines default to defaultFileHeadLines.
func (s *Server) WorkspaceGetFileHead(ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	rel, err := resolveInRoot(root, in.Path)
	if err != nil {
		return nil, err
	}
	lines := in.Lines
	if lines <= 0 {
		lines = defaultFileHeadLines
	}
	out, err := fs.NewLocalExecutor(root).Read(ctx, fs.ReadInput{Path: rel, Limit: lines})
	if err != nil {
		return nil, err
	}
	return &protocol.FileHead{Path: in.Path, Lines: fileLines(out)}, nil
}

// fileLines splits a Read result into numbered preview lines. StartLine is
// 0-based; the wire LineNumber is 1-based. A read that windowed nothing (an
// empty file) yields no lines rather than one spurious blank.
func fileLines(out fs.ReadOutput) []protocol.FileLine {
	if out.Content == "" && out.TotalLines == 0 {
		return []protocol.FileLine{}
	}
	parts := strings.Split(out.Content, "\n")
	lines := make([]protocol.FileLine, 0, len(parts))
	for i, text := range parts {
		lines = append(lines, protocol.FileLine{LineNumber: out.StartLine + i + 1, Text: text})
	}
	return lines
}

// defaultGrepLimit caps workspace.grep matches when the client gives no
// (or a non-positive) limit.
const defaultGrepLimit = 100

// WorkspaceGrep runs a regex search under the workspace root (API.md §7.5),
// optionally scoped to a sub-path. Matches are capped at limit; Total is
// self-describing per §7.5's no-silent-caps rule — when the capped search
// truncates, a count-mode pass recovers the true total so Total > len(Matches)
// signals "more exist" rather than under-reporting.
func (s *Server) WorkspaceGrep(ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
	if in.Query == "" {
		return nil, fmt.Errorf("%w: query required", protocol.ErrInvalidParams)
	}
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	searchRoot := root
	if in.Path != "" {
		rel, err := resolveInRoot(root, in.Path)
		if err != nil {
			return nil, err
		}
		searchRoot = filepath.Join(root, rel)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultGrepLimit
	}

	exec := fs.NewLocalExecutor(root)
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: in.Query, Root: searchRoot, MaxResults: limit})
	if err != nil {
		return nil, err
	}
	matches := make([]protocol.GrepMatch, 0, len(out.Matches))
	for _, m := range out.Matches {
		matches = append(matches, protocol.GrepMatch{Path: m.Path, LineNumber: m.Line, Text: m.Text})
	}

	total := len(matches)
	if out.Truncated {
		// The capped content search hid some hits; a count-mode pass gives the
		// honest total so the client sees total > len(matches) and knows to
		// narrow the query. Best-effort: if the count pass fails, fall back to
		// "at least one more" so the total is never overstated.
		if n, cerr := grepTotal(ctx, exec, in.Query, searchRoot); cerr == nil && n > total {
			total = n
		} else if total == limit {
			total = limit + 1
		}
	}
	return &protocol.GrepResult{Matches: matches, Total: total}, nil
}

// grepTotal counts every match for pattern under root (uncapped count mode),
// summing the per-file counts into one total.
func grepTotal(ctx context.Context, exec fs.Executor, pattern, root string) (int, error) {
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: pattern, Root: root, OutputMode: fs.GrepOutputCount})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, c := range out.Counts {
		total += c.Count
	}
	return total, nil
}
