package protocol

// WorkspaceQuery is the common explicit scope for workspace reads (API.md §7.5).
type WorkspaceQuery struct {
	Workspace WorkspaceRef `json:"workspace"`
}

// GetFileHeadRequest — workspace.files.head body.
type GetFileHeadRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path"`
	Lines     int          `json:"lines,omitempty"`
}

// GrepRequest — workspace.files.search body. Query is a Go/RE2-compatible
// regular expression of at most 64 KiB. A zero Limit selects 100 retained
// matches; larger values are capped at 1000. The service may retain fewer rows
// when the 8 MiB result-material budget is reached while still reporting the
// exact Total for its admitted text corpus.
type GrepRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Query     string       `json:"query"`
	Path      string       `json:"path,omitempty"`
	Limit     int          `json:"limit,omitempty"`
}

// ListFilesRequest — workspace.files.list body (API.md §7.5). Lists files under
// Path (relative to CWD, jailed). Recursive (or a Glob) yields a flat subtree
// file list — the @file / fuzzy source; otherwise the immediate children — the
// lazy file-tree level. .gitignore + backstop excludes apply unless
// IncludeIgnored. PageQuery carries stable cursor pagination.
type ListFilesRequest struct {
	Workspace      WorkspaceRef `json:"workspace"`
	Path           string       `json:"path,omitempty"`
	Glob           string       `json:"glob,omitempty"`
	Recursive      bool         `json:"recursive,omitempty"`
	IncludeIgnored bool         `json:"includeIgnored,omitempty"`
	PageQuery
}

// ReadFileRequest — workspace.files.read body (API.md §7.5). Reads the whole
// file, or the StartLine..EndLine window (1-based inclusive, editor-facing)
// when given. A zero MaxBytes selects the 1 MiB default; larger values are
// capped at 8 MiB. FileContent.Truncated reports omitted source material.
type ReadFileRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path"`
	StartLine int          `json:"startLine,omitempty"`
	EndLine   int          `json:"endLine,omitempty"`
	MaxBytes  int          `json:"maxBytes,omitempty"`
}

// FileContent is the workspace.files.read result (API.md §7.5). TotalLines is the
// whole-file line count even for a windowed read (so the UI can show "12–40 /
// 320"). StartLine/EndLine describe the served window (1-based inclusive), set
// only when a range was requested; a byte-limited last line may be a valid
// UTF-8 prefix of that source line.
type FileContent struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"` // always "utf-8" (binary files error)
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	EndLine    int    `json:"endLine,omitempty"`
}

// FileEntryType is a listed entry's kind (workspace.files.list, API.md §7.5).
type FileEntryType string

const (
	FileEntryFile    FileEntryType = "file"
	FileEntryDir     FileEntryType = "dir"
	FileEntrySymlink FileEntryType = "symlink"
)

// FileEntry is one inspected entry in workspace.files.list (API.md §7.5). Path
// is relative to the workspace root; type, size, and modification time come
// from one inspection of that entry.
type FileEntry struct {
	Path       string        `json:"path"`
	Name       string        `json:"name"`
	Type       FileEntryType `json:"type"`
	SizeBytes  *int64        `json:"sizeBytes,omitempty"`
	ModifiedAt string        `json:"modifiedAt"`
}

// FileHead is a file preview (API.md §4.5).
type FileHead struct {
	Path  string     `json:"path"`
	Lines []FileLine `json:"lines"`
}

// FileLine is one preview line — plain text, client highlights (API.md §4.5).
type FileLine struct {
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}

// GrepResult is the workspace.files.search result (API.md §4.5). Matches is a
// stable whole-line prefix; Total is the exact count across admitted UTF-8 text
// files and may exceed len(Matches) when count or material limits apply.
type GrepResult struct {
	Matches []GrepMatch `json:"matches"`
	Total   int         `json:"total"`
}

// GrepMatch is one grep hit — plain text (API.md §4.5).
type GrepMatch struct {
	Path       string `json:"path"`
	LineNumber int    `json:"lineNumber"`
	Text       string `json:"text"`
}
