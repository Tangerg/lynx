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

func (kind FileType) Valid() bool {
	return kind == FileEntryFile || kind == FileEntryDirectory || kind == FileEntrySymlink
}

type FileEntry struct {
	Path       string
	Name       string
	Type       FileType
	SizeBytes  *int64
	ModifiedAt string
}

func (entry FileEntry) Validate() error {
	switch {
	case strings.TrimSpace(entry.Path) == "":
		return errors.New("file entry path is empty")
	case strings.TrimSpace(entry.Name) == "":
		return errors.New("file entry name is empty")
	case !entry.Type.Valid():
		return fmt.Errorf("file entry type %q is invalid", entry.Type)
	case entry.SizeBytes != nil && *entry.SizeBytes < 0:
		return errors.New("file entry size is negative")
	default:
		return nil
	}
}

type FilePage struct {
	Entries    []FileEntry
	NextCursor string
}

func (page FilePage) Validate() error {
	for index, entry := range page.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("file entry %d: %w", index, err)
		}
	}
	return nil
}

type FilesRequest struct {
	Workspace      string
	Path           string
	Glob           string
	Recursive      bool
	IncludeIgnored bool
	Limit          int
	Cursor         string
}

func (request FilesRequest) Validate() error {
	if strings.TrimSpace(request.Workspace) == "" {
		return errors.New("file list workspace is empty")
	}
	if request.Limit < 0 {
		return errors.New("file list limit is negative")
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

func (request ReadRequest) Validate() error {
	switch {
	case strings.TrimSpace(request.Workspace) == "":
		return errors.New("file read workspace is empty")
	case strings.TrimSpace(request.Path) == "":
		return errors.New("file read path is empty")
	case request.StartLine < 0 || request.EndLine < 0 || request.MaxBytes < 0:
		return errors.New("file read bounds cannot be negative")
	case request.EndLine > 0 && request.StartLine == 0:
		return errors.New("file read end line requires a start line")
	case request.StartLine > 0 && request.EndLine > 0 && request.EndLine < request.StartLine:
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

func (content FileContent) Validate() error {
	switch {
	case strings.TrimSpace(content.Path) == "":
		return errors.New("file content path is empty")
	case content.Encoding != "utf-8":
		return fmt.Errorf("file content encoding %q is unsupported", content.Encoding)
	case content.TotalLines < 0:
		return errors.New("file content line count is negative")
	case content.StartLine < 0 || content.EndLine < 0:
		return errors.New("file content window is negative")
	case content.EndLine > 0 && content.StartLine == 0:
		return errors.New("file content end line has no start line")
	case content.StartLine > 0 && content.EndLine > 0 && content.EndLine < content.StartLine:
		return errors.New("file content window is reversed")
	default:
		return nil
	}
}

func (content FileContent) Window() string {
	if content.StartLine == 0 {
		return fmt.Sprintf("%d lines", content.TotalLines)
	}
	return fmt.Sprintf("lines %d-%d/%d", content.StartLine, content.EndLine, content.TotalLines)
}

type HeadRequest struct {
	Workspace string
	Path      string
	Lines     int
}

func (request HeadRequest) Validate() error {
	if strings.TrimSpace(request.Workspace) == "" || strings.TrimSpace(request.Path) == "" {
		return errors.New("file head requires workspace and path")
	}
	if request.Lines < 0 {
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

func (head FileHead) Validate() error {
	if strings.TrimSpace(head.Path) == "" {
		return errors.New("file head path is empty")
	}
	previous := 0
	for index, line := range head.Lines {
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

func (request SearchRequest) Validate() error {
	if strings.TrimSpace(request.Workspace) == "" || strings.TrimSpace(request.Query) == "" {
		return errors.New("workspace search requires workspace and query")
	}
	if request.Limit < 0 {
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

func (result SearchResult) Validate() error {
	if result.Total < len(result.Matches) {
		return errors.New("workspace search total is smaller than its matches")
	}
	for index, match := range result.Matches {
		if strings.TrimSpace(match.Path) == "" || match.Line <= 0 {
			return fmt.Errorf("workspace search match %d is invalid", index)
		}
	}
	return nil
}
