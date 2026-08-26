package workspace

import (
	"errors"
	"fmt"
	"strings"
)

type FileType string

const (
	FileEntryFile      FileType = "file"
	FileEntryDirectory FileType = "directory"
	FileEntrySymlink   FileType = "symlink"
)

func (f FileType) Valid() bool {
	return f == FileEntryFile || f == FileEntryDirectory || f == FileEntrySymlink
}

type FileEntry struct {
	Path       string
	Name       string
	Type       FileType
	SizeBytes  *int64
	ModifiedAt string
}

func (f FileEntry) Validate() error {
	switch {
	case strings.TrimSpace(f.Path) == "":
		return errors.New("file entry path is empty")
	case strings.TrimSpace(f.Name) == "":
		return errors.New("file entry name is empty")
	case !f.Type.Valid():
		return fmt.Errorf("file entry type %q is invalid", f.Type)
	case f.SizeBytes != nil && *f.SizeBytes < 0:
		return errors.New("file entry size is negative")
	default:
		return nil
	}
}

type FileListing struct {
	Entries []FileEntry
}

func (f FileListing) Validate() error {
	paths := make(map[string]struct{}, len(f.Entries))
	for index, entry := range f.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("file entry %d: %w", index, err)
		}
		if _, exists := paths[entry.Path]; exists {
			return fmt.Errorf("file entry %d repeats path %q", index, entry.Path)
		}
		paths[entry.Path] = struct{}{}
	}
	return nil
}

type FilesRequest struct {
	Workspace      string
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
}

func (f FilesRequest) Validate() error {
	if strings.TrimSpace(f.Workspace) == "" {
		return errors.New("file list workspace is empty")
	}
	return nil
}

type ReadRequest struct {
	Workspace string
	Path      string
	StartLine int
	EndLine   int
	MaxBytes  int
}

func (r ReadRequest) Validate() error {
	switch {
	case strings.TrimSpace(r.Workspace) == "":
		return errors.New("file read workspace is empty")
	case strings.TrimSpace(r.Path) == "":
		return errors.New("file read path is empty")
	case r.StartLine < 0 || r.EndLine < 0 || r.MaxBytes < 0:
		return errors.New("file read bounds cannot be negative")
	case r.EndLine > 0 && r.StartLine == 0:
		return errors.New("file read end line requires a start line")
	case r.StartLine > 0 && r.EndLine > 0 && r.EndLine < r.StartLine:
		return errors.New("file read end line precedes start line")
	default:
		return nil
	}
}

type FileContent struct {
	Path       string
	Content    string
	Encoding   string
	TotalLines int
	Truncated  bool
	StartLine  int
	EndLine    int
}

func (f FileContent) Validate() error {
	switch {
	case strings.TrimSpace(f.Path) == "":
		return errors.New("file content path is empty")
	case f.Encoding != "utf-8":
		return fmt.Errorf("file content encoding %q is unsupported", f.Encoding)
	case f.TotalLines < 0:
		return errors.New("file content line count is negative")
	case f.StartLine < 0 || f.EndLine < 0:
		return errors.New("file content window is negative")
	case f.EndLine > 0 && f.StartLine == 0:
		return errors.New("file content end line has no start line")
	case f.StartLine > 0 && f.EndLine > 0 && f.EndLine < f.StartLine:
		return errors.New("file content window is reversed")
	default:
		return nil
	}
}

func (f FileContent) Window() string {
	if f.StartLine == 0 {
		return fmt.Sprintf("%d lines", f.TotalLines)
	}
	return fmt.Sprintf("lines %d-%d/%d", f.StartLine, f.EndLine, f.TotalLines)
}

type HeadRequest struct {
	Workspace string
	Path      string
	Lines     int
}

func (h HeadRequest) Validate() error {
	if strings.TrimSpace(h.Workspace) == "" || strings.TrimSpace(h.Path) == "" {
		return errors.New("file head requires workspace and path")
	}
	if h.Lines < 0 {
		return errors.New("file head line count is negative")
	}
	return nil
}

type FileLine struct {
	Number int
	Text   string
}

type FileHead struct {
	Path  string
	Lines []FileLine
}

func (f FileHead) Validate() error {
	if strings.TrimSpace(f.Path) == "" {
		return errors.New("file head path is empty")
	}
	previous := 0
	for index, line := range f.Lines {
		if line.Number <= previous {
			return fmt.Errorf("file head line %d is not strictly ordered", index)
		}
		previous = line.Number
	}
	return nil
}

type SearchRequest struct {
	Workspace string
	Query     string
	Path      string
	Limit     int
}

func (s SearchRequest) Validate() error {
	if strings.TrimSpace(s.Workspace) == "" || strings.TrimSpace(s.Query) == "" {
		return errors.New("workspace search requires workspace and query")
	}
	if s.Limit < 0 {
		return errors.New("workspace search limit is negative")
	}
	return nil
}

type Match struct {
	Path string
	Line int
	Text string
}

type SearchResult struct {
	Matches []Match
	Total   int
}

func (s SearchResult) Validate() error {
	if s.Total < len(s.Matches) {
		return errors.New("workspace search total is smaller than its matches")
	}
	for index, match := range s.Matches {
		if strings.TrimSpace(match.Path) == "" || match.Line <= 0 {
			return fmt.Errorf("workspace search match %d is invalid", index)
		}
	}
	return nil
}
