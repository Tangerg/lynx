package protocol

// DiffMode selects the baseline workspace.diff.get compares against (AUX_API §2.3).
type DiffMode string

const (
	DiffModeWorktree DiffMode = "worktree" // changes vs HEAD, incl. untracked (default)
	DiffModeBase     DiffMode = "base"     // vs merge-base with the default branch
)

// DiffFormat selects the workspace.diff.get result shape (AUX_API §2.3).
type DiffFormat string

const (
	DiffFormatRows DiffFormat = "rows" // per-file structured diff (default)
	DiffFormatRaw  DiffFormat = "raw"  // unified patch string
)

// GetDiffRequest — workspace.diff.get body (AUX_API §2.3). Mode selects the
// baseline (worktree=changes vs HEAD incl. untracked; base=vs merge-base with
// default branch). Format selects the shape (rows=structured; raw=unified patch
// string). Limit caps the diff rows (rows format); over it the result is
// truncated at a file boundary (Diff.Truncated) rather than silently dropped.
type GetDiffRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
	Path      string       `json:"path,omitempty"`
	Mode      DiffMode     `json:"mode,omitempty"`   // "worktree" (default) | "base"
	Format    DiffFormat   `json:"format,omitempty"` // "rows" (default) | "raw"
	Limit     int          `json:"limit,omitempty"`
}

// Diff is the workspace.diff.get result (AUX_API §2.3) — a sum type: Files is
// populated for format=rows (per-file structured diff), Patch for format=raw
// (the unified patch string). Truncated self-describes a row-limit cut at a
// file boundary ("no silent caps", §7.5).
type Diff struct {
	Files     []FileDiff `json:"files,omitempty"`
	Patch     string     `json:"patch,omitempty"`
	Truncated bool       `json:"truncated,omitempty"`
}

// FileStatus is the past-tense working-tree status vocabulary shared by
// WorkspaceFileChange and FileDiff (§4.5).
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileDiff is one file's structured diff (AUX_API §2.3). Added/Removed are
// omitted for a Binary file (Rows empty) rather than reported as a fake 0;
// PreviousPath is set only for renames.
type FileDiff struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitempty"`
	Removed      *int       `json:"removed,omitempty"`
	Binary       bool       `json:"binary,omitempty"`
	Rows         []DiffRow  `json:"rows"`
}

// WorkspaceFileChange is one entry in workspace.changes.list (AUX_API §2.2).
// Added/Removed are omitted for a Binary file (not a fake 0); PreviousPath is
// set only for renames.
type WorkspaceFileChange struct {
	Path         string     `json:"path"`
	Status       FileStatus `json:"status"` // see FileStatus
	PreviousPath string     `json:"previousPath,omitempty"`
	Added        *int       `json:"added,omitempty"`
	Removed      *int       `json:"removed,omitempty"`
	Binary       bool       `json:"binary,omitempty"`
}
