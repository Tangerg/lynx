package protocol

// WorkspaceQuery is the common cwd input for workspace reads (API.md §7.5).
type WorkspaceQuery struct {
	Cwd string `json:"cwd,omitempty"`
}

// GetFileHeadRequest — workspace.getFileHead body.
type GetFileHeadRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Path  string `json:"path"`
	Lines int    `json:"lines,omitempty"`
}

// GrepRequest — workspace.grep body.
type GrepRequest struct {
	Cwd   string `json:"cwd,omitempty"`
	Query string `json:"query"`
	Path  string `json:"path,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// ListFilesRequest — workspace.listFiles body (API.md §7.5). Lists files under
// Path (relative to Cwd, jailed). Recursive (or a Glob) yields a flat subtree
// file list — the @file / fuzzy source; otherwise the immediate children — the
// lazy file-tree level. .gitignore + backstop excludes apply unless
// IncludeIgnored. PageQuery carries stable cursor pagination.
type ListFilesRequest struct {
	Cwd            string `json:"cwd,omitempty"`
	Path           string `json:"path,omitempty"`
	Glob           string `json:"glob,omitempty"`
	Recursive      bool   `json:"recursive,omitempty"`
	IncludeIgnored bool   `json:"includeIgnored,omitempty"`
	PageQuery
}

// ReadFileRequest — workspace.readFile body (API.md §7.5). Reads the whole
// file, or the StartLine..EndLine window (1-based inclusive, editor-facing)
// when given. MaxBytes caps an over-large read (the executor self-describes the
// cut via FileContent.Truncated).
type ReadFileRequest struct {
	Cwd       string `json:"cwd,omitempty"`
	Path      string `json:"path"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	MaxBytes  int    `json:"maxBytes,omitempty"`
}

// FileContent is the workspace.readFile result (API.md §7.5). TotalLines is the
// whole-file line count even for a windowed read (so the UI can show "12–40 /
// 320"). StartLine/EndLine echo the served window (1-based inclusive), set only
// when a range was requested.
type FileContent struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"` // always "utf-8" (binary files error)
	TotalLines int    `json:"totalLines"`
	Truncated  bool   `json:"truncated,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	EndLine    int    `json:"endLine,omitempty"`
}

// FileEntryType is a listed entry's kind (workspace.listFiles, API.md §7.5).
type FileEntryType string

const (
	FileEntryFile    FileEntryType = "file"
	FileEntryDir     FileEntryType = "dir"
	FileEntrySymlink FileEntryType = "symlink"
)

// FileEntry is one inspected entry in workspace.listFiles (API.md §7.5). Path
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

// GrepResult is the workspace.grep result (API.md §4.5). Total may exceed
// len(Matches) when limited.
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
