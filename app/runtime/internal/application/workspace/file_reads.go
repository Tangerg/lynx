package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/pagination"
)

// Files owns root-scoped file browser operations.
type Files struct {
	scope *Scope
	files FileBrowser
}

func NewFiles(scope *Scope, files FileBrowser) *Files {
	return &Files{scope: scope, files: files}
}

// FileEntryKind identifies a file-system entry in the workspace browser.
type FileEntryKind string

const (
	FileEntryFile    FileEntryKind = "file"
	FileEntryDir     FileEntryKind = "dir"
	FileEntrySymlink FileEntryKind = "symlink"
)

// FileEntry is an inspected, root-relative workspace entry.
type FileEntry struct {
	Path       string
	Name       string
	Kind       FileEntryKind
	SizeBytes  int64
	ModifiedAt time.Time
}

func (e FileEntry) orderKey() string {
	class := "1"
	if e.Kind == FileEntryDir {
		class = "0"
	}
	return class + ":" + e.Path
}

// FileListOptions controls one workspace file listing.
type FileListOptions struct {
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

// FileBrowser is the application-owned port for workspace file reads.
type FileBrowser interface {
	List(ctx context.Context, root string, options FileListOptions) ([]FileEntry, error)
	Read(ctx context.Context, root string, input FileReadInput) (FileReadResult, error)
	Grep(ctx context.Context, root string, input GrepInput) (GrepResult, error)
}

// FileListInput is one paged workspace listing request.
type FileListInput struct {
	CWD string
	FileListOptions
	Cursor string
	Limit  int
}

// FilePage is a stable cursor page of workspace entries.
type FilePage struct {
	Entries    []FileEntry
	NextCursor string
}

// FileReadInput specifies a root-relative file read. StartLine/EndLine are
// one-based and inclusive when StartLine is positive.
type FileReadInput struct {
	Path      string
	MaxBytes  int
	StartLine int
	EndLine   int
}

// FileReadResult is a file read with whole-file line information.
type FileReadResult struct {
	Content    string
	TotalLines int
	StartLine  int // zero-based
	EndLine    int // one-based inclusive
	Truncated  bool
}

// GrepInput specifies one root-scoped content search.
type GrepInput struct {
	Path  string
	Query string
	Limit int
}

// GrepMatch is one content-search match.
type GrepMatch struct {
	Path       string
	LineNumber int
	Text       string
}

// GrepResult is a capped content search plus its honest total match count.
type GrepResult struct {
	Matches []GrepMatch
	Total   int
}

// FileHead is the leading numbered text of one file.
type FileHead struct {
	Lines []FileLine
}

// FileLine is one one-based line in a file preview.
type FileLine struct {
	Number int
	Text   string
}

const (
	defaultFileListPageLimit = 1000
	defaultFileHeadLines     = 200
	defaultGrepLimit         = 100
	fileListPageNamespace    = "workspace.files"
)

// List returns one stable cursor page of entries below a workspace root.
func (f *Files) List(ctx context.Context, input FileListInput) (FilePage, error) {
	root, err := f.scope.root(input.CWD)
	if err != nil {
		return FilePage{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = f.scope.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return FilePage{}, err
		}
	}
	if f.files == nil {
		return FilePage{}, errors.New("workspace: file browser is not configured")
	}
	entries, err := f.files.List(ctx, root, FileListOptions{
		Path: path, Glob: input.Glob, Recursive: input.Recursive, IncludeIgnored: input.IncludeIgnored,
	})
	if err != nil {
		return FilePage{}, err
	}
	filters := []string{
		root,
		path,
		input.Glob,
		strconv.FormatBool(input.Recursive),
		strconv.FormatBool(input.IncludeIgnored),
	}
	page, next, err := pageFileEntries(entries, filters, input.Cursor, input.Limit)
	if err != nil {
		return FilePage{}, err
	}
	return FilePage{Entries: page, NextCursor: next}, nil
}

// Head returns the first requested lines of one workspace file.
func (f *Files) Head(ctx context.Context, cwd, path string, lines int) (FileHead, error) {
	root, err := f.scope.root(cwd)
	if err != nil {
		return FileHead{}, err
	}
	if path == "" {
		return FileHead{}, ErrPathRequired
	}
	path, err = f.scope.paths.ResolveExistingInRoot(root, path)
	if err != nil {
		return FileHead{}, err
	}
	if lines <= 0 {
		lines = defaultFileHeadLines
	}
	if f.files == nil {
		return FileHead{}, errors.New("workspace: file browser is not configured")
	}
	read, err := f.files.Read(ctx, root, FileReadInput{Path: path, EndLine: lines, StartLine: 1})
	if err != nil {
		return FileHead{}, err
	}
	return FileHead{Lines: previewLines(read)}, nil
}

// Read returns all or a one-based inclusive line window of a workspace
// file. It validates ranges before invoking the filesystem port.
func (f *Files) Read(ctx context.Context, cwd string, input FileReadInput) (FileReadResult, error) {
	if err := input.validate(); err != nil {
		return FileReadResult{}, err
	}
	root, err := f.scope.root(cwd)
	if err != nil {
		return FileReadResult{}, err
	}
	input.Path, err = f.scope.paths.ResolveExistingInRoot(root, input.Path)
	if err != nil {
		return FileReadResult{}, err
	}
	if f.files == nil {
		return FileReadResult{}, errors.New("workspace: file browser is not configured")
	}
	return f.files.Read(ctx, root, input)
}

func (input FileReadInput) validate() error {
	if input.Path == "" {
		return ErrPathRequired
	}
	if input.StartLine < 0 || input.EndLine < 0 || input.MaxBytes < 0 {
		return ErrInvalidFileRange
	}
	if input.EndLine > 0 && input.StartLine == 0 {
		return ErrInvalidFileRange
	}
	if input.StartLine > 0 && input.EndLine > 0 && input.EndLine < input.StartLine {
		return ErrInvalidFileRange
	}
	return nil
}

// Grep searches a workspace root or an existing subdirectory. A truncated
// search returns an honest total rather than silently under-reporting hits.
func (f *Files) Grep(ctx context.Context, cwd string, input GrepInput) (GrepResult, error) {
	if input.Query == "" {
		return GrepResult{}, ErrGrepQueryMissing
	}
	root, err := f.scope.root(cwd)
	if err != nil {
		return GrepResult{}, err
	}
	if input.Path != "" {
		input.Path, err = f.scope.paths.ResolveExistingInRoot(root, input.Path)
		if err != nil {
			return GrepResult{}, err
		}
	}
	if input.Limit <= 0 {
		input.Limit = defaultGrepLimit
	}
	if f.files == nil {
		return GrepResult{}, errors.New("workspace: file browser is not configured")
	}
	return f.files.Grep(ctx, root, input)
}

func previewLines(read FileReadResult) []FileLine {
	if read.Content == "" && read.TotalLines == 0 {
		return []FileLine{}
	}
	parts := strings.Split(read.Content, "\n")
	lines := make([]FileLine, 0, len(parts))
	for index, text := range parts {
		lines = append(lines, FileLine{Number: read.StartLine + index + 1, Text: text})
	}
	return lines
}

func pageFileEntries(entries []FileEntry, filters []string, cursor string, limit int) ([]FileEntry, string, error) {
	size, err := pagination.Limit(limit, defaultFileListPageLimit)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrPageLimit, err)
	}
	slices.SortFunc(entries, func(a, b FileEntry) int { return cmp.Compare(a.orderKey(), b.orderKey()) })
	anchor, err := pagination.Decode(cursor, fileListPageNamespace, filters)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrPageCursor, err)
	}
	if len(anchor) > 0 {
		if len(anchor) != 1 {
			return nil, "", ErrPageCursor
		}
		start := len(entries)
		for index, entry := range entries {
			if candidate := entry.orderKey(); candidate >= anchor[0] {
				start = index
				if candidate == anchor[0] {
					start++
				}
				break
			}
		}
		entries = entries[start:]
	}
	end := min(size, len(entries))
	page := slices.Clone(entries[:end])
	if end == len(entries) {
		return page, "", nil
	}
	return page, pagination.Encode(fileListPageNamespace, filters, []string{entries[end-1].orderKey()}), nil
}
