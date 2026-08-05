package workspace

import "context"

// VCS owns root-scoped Git status and diff operations.
type VCS struct {
	scope *Scope
	git   GitReader
}

func NewVCS(scope *Scope, git GitReader) *VCS { return &VCS{scope: scope, git: git} }

// FileStatus is the application vocabulary for a working-tree change.
type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

// FileChange is one working-tree change.
type FileChange struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
}

// DiffRowType is the application vocabulary for a parsed unified-diff row.
type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

// DiffRow is one structured diff row.
type DiffRow struct {
	Type      DiffRowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}

// FileDiff is one file's structured diff.
type FileDiff struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Binary       bool
	Added        int
	Removed      int
	Rows         []DiffRow
}

// GitReader is the application-owned port for working-tree status and diff
// reads. Its error contract uses this package's VCS sentinels.
type GitReader interface {
	Changes(ctx context.Context, root string) ([]FileChange, error)
	StructuredDiff(ctx context.Context, root, path string, base bool) ([]FileDiff, error)
	RawDiff(ctx context.Context, root, path string, base bool) (string, error)
}

// DiffInput selects a working-tree or merge-base diff, optionally as raw text.
type DiffInput struct {
	CWD   string
	Path  string
	Base  bool
	Raw   bool
	Limit int
}

// Diff is a structured or raw workspace diff.
type Diff struct {
	Patch     string
	Files     []FileDiff
	Truncated bool
}

// Changes reads the root's VCS status.
func (v *VCS) Changes(ctx context.Context, cwd string) ([]FileChange, error) {
	root, err := v.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if v.git == nil {
		return nil, ErrVCSUnavailable
	}
	return v.git.Changes(ctx, root)
}

// Diff reads a workspace VCS diff, keeping path confinement and file-boundary
// truncation in the application use case.
func (v *VCS) Diff(ctx context.Context, input DiffInput) (Diff, error) {
	root, err := v.scope.root(input.CWD)
	if err != nil {
		return Diff{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = v.scope.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return Diff{}, err
		}
	}
	if v.git == nil {
		return Diff{}, ErrVCSUnavailable
	}
	if input.Raw {
		patch, err := v.git.RawDiff(ctx, root, path, input.Base)
		return Diff{Patch: patch}, err
	}
	files, err := v.git.StructuredDiff(ctx, root, path, input.Base)
	if err != nil {
		return Diff{}, err
	}
	files, truncated := limitDiffRows(files, input.Limit)
	return Diff{Files: files, Truncated: truncated}, nil
}

func limitDiffRows(files []FileDiff, limit int) ([]FileDiff, bool) {
	if limit <= 0 {
		return files, false
	}
	rows := 0
	for index, file := range files {
		if index > 0 && rows+len(file.Rows) > limit {
			return files[:index], true
		}
		rows += len(file.Rows)
	}
	return files, false
}
