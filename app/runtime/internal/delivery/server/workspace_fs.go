package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/tools/fs"
)

// workspace.* filesystem-backed reads (API.md §7.5): file listing, file
// preview, and grep — all jailed to the workspace root.

// WorkspaceListFiles lists files under a cwd-relative path (API.md §7.5),
// jailed to the workspace root. Recursive (or a glob) yields a flat subtree
// file list — the @file / fuzzy source; otherwise the immediate children — the
// lazy file-tree level. gitignore-aware in a repo, backstop-filtered otherwise.
// Not gated: a basic read like getFileHead. Results use stable cursor pagination;
// an oversized candidate set fails clearly instead of looking complete.
func (s *Server) WorkspaceListFiles(ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	relPath := ""
	if in.Path != "" {
		if relPath, err = resolveInRoot(root, in.Path); err != nil {
			return nil, err
		}
	}
	entries, err := workspace.ListFiles(ctx, root, workspace.ListFilesOptions{
		Path:           relPath,
		Glob:           in.Glob,
		Recursive:      in.Recursive,
		IncludeIgnored: in.IncludeIgnored,
	})
	if errors.Is(err, workspace.ErrListingTooLarge) {
		return nil, fmt.Errorf("%w: file listing is too large; narrow path", protocol.ErrInvalidParams)
	}
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	page, next, err := pageOrderedByCursor(entries, workspace.FileEntry.OrderKey, in.Cursor, in.Limit, defaultFileListPageLimit)
	if err != nil {
		return nil, err
	}
	data := make([]protocol.FileEntry, 0, len(page))
	for _, entry := range page {
		var sizeBytes *int64
		if entry.Kind == workspace.EntryFile {
			sizeBytes = &entry.SizeBytes
		}
		data = append(data, protocol.FileEntry{
			Path:       entry.Path,
			Name:       entry.Name,
			Type:       protocol.FileEntryType(entry.Kind),
			SizeBytes:  sizeBytes,
			ModifiedAt: entry.ModifiedAt.Format(time.RFC3339Nano),
		})
	}
	return &protocol.Page[protocol.FileEntry]{Data: data, NextCursor: next}, nil
}

const defaultFileListPageLimit = 1000

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
	rel, err := resolveExistingInRoot(root, in.Path)
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

// WorkspaceReadFile returns a cwd-relative file's text (API.md §7.5), jailed to
// the workspace root. Reads the whole file, or the StartLine..EndLine window
// (1-based inclusive) when given; TotalLines is the whole-file count regardless.
// Binary files surface fs.ErrBinaryFile. Not gated — a basic read like
// getFileHead.
func (s *Server) WorkspaceReadFile(ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
	if err := validateReadFileRequest(in); err != nil {
		return nil, err
	}
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	rel, err := resolveExistingInRoot(root, in.Path)
	if err != nil {
		return nil, err
	}
	read := fs.ReadInput{Path: rel, MaxBytes: in.MaxBytes}
	windowed := in.StartLine > 0
	if windowed {
		read.Offset = in.StartLine - 1
		if in.EndLine >= in.StartLine {
			read.Limit = in.EndLine - in.StartLine + 1
		}
	}
	out, err := fs.NewLocalExecutor(root).Read(ctx, read)
	if err != nil {
		return nil, err
	}
	fc := &protocol.FileContent{
		Path:       in.Path,
		Content:    out.Content,
		Encoding:   "utf-8",
		TotalLines: out.TotalLines,
		Truncated:  out.Truncated,
	}
	if windowed {
		fc.StartLine = out.StartLine + 1 // ReadOutput line indices are 0-based
		fc.EndLine = out.EndLine
	}
	return fc, nil
}

func validateReadFileRequest(in protocol.ReadFileRequest) error {
	if in.StartLine < 0 || in.EndLine < 0 {
		return fmt.Errorf("%w: startLine and endLine must be non-negative", protocol.ErrInvalidParams)
	}
	if in.MaxBytes < 0 {
		return fmt.Errorf("%w: maxBytes must be non-negative", protocol.ErrInvalidParams)
	}
	if in.EndLine > 0 && in.StartLine == 0 {
		return fmt.Errorf("%w: endLine requires startLine", protocol.ErrInvalidParams)
	}
	if in.StartLine > 0 && in.EndLine > 0 && in.EndLine < in.StartLine {
		return fmt.Errorf("%w: endLine must be greater than or equal to startLine", protocol.ErrInvalidParams)
	}
	return nil
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
		rel, err := resolveExistingInRoot(root, in.Path)
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
