package workspace

import "context"

// FileStatus is the application vocabulary for a working-tree change. It is
// intentionally independent of both the Git adapter's type and the wire enum.
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
	ListFileChanges(ctx context.Context, root string) ([]FileChange, error)
	StructuredDiff(ctx context.Context, root, path string, base bool) ([]FileDiff, error)
	RawDiff(ctx context.Context, root, path string, base bool) (string, error)
}

// DiffInput selects a working-tree or merge-base diff, optionally as raw text.
type DiffInput struct {
	Cwd   string
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

// ListFileChanges reads the root's VCS status.
func (c *VCS) ListFileChanges(ctx context.Context, cwd string) ([]FileChange, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.git == nil {
		return nil, ErrVCSUnavailable
	}
	return c.git.ListFileChanges(ctx, root)
}

// Diff reads a workspace VCS diff, keeping path confinement and file-boundary
// truncation in the application use case rather than the delivery projection.
func (c *VCS) Diff(ctx context.Context, input DiffInput) (Diff, error) {
	root, err := c.context.root(input.Cwd)
	if err != nil {
		return Diff{}, err
	}
	path := ""
	if input.Path != "" {
		path, err = c.context.paths.ResolveInRoot(root, input.Path)
		if err != nil {
			return Diff{}, err
		}
	}
	if c.git == nil {
		return Diff{}, ErrVCSUnavailable
	}
	if input.Raw {
		patch, err := c.git.RawDiff(ctx, root, path, input.Base)
		return Diff{Patch: patch}, err
	}
	files, err := c.git.StructuredDiff(ctx, root, path, input.Base)
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
