package workspace

import (
	"errors"
	"fmt"
	"strings"
)

type DiffMode string

const (
	DiffModeWorktree DiffMode = "worktree"
	DiffModeBase     DiffMode = "base"
)

type DiffFormat string

const (
	DiffFormatRows DiffFormat = "rows"
	DiffFormatRaw  DiffFormat = "raw"
)

type FileStatus string

const (
	FileStatusAdded     FileStatus = "added"
	FileStatusModified  FileStatus = "modified"
	FileStatusDeleted   FileStatus = "deleted"
	FileStatusRenamed   FileStatus = "renamed"
	FileStatusUntracked FileStatus = "untracked"
)

func (f FileStatus) Valid() bool {
	switch f {
	case FileStatusAdded, FileStatusModified, FileStatusDeleted, FileStatusRenamed, FileStatusUntracked:
		return true
	default:
		return false
	}
}

type Change struct {
	Path         string
	Status       FileStatus
	PreviousPath string
	Added        *int
	Removed      *int
	Binary       bool
}

func (c Change) Validate() error {
	switch {
	case strings.TrimSpace(c.Path) == "":
		return errors.New("changed file path is empty")
	case !c.Status.Valid():
		return fmt.Errorf("changed file status %q is invalid", c.Status)
	case c.Status == FileStatusRenamed && strings.TrimSpace(c.PreviousPath) == "":
		return errors.New("renamed file has no previous path")
	case c.Status != FileStatusRenamed && c.PreviousPath != "":
		return errors.New("non-renamed file has a previous path")
	case c.Binary && (c.Added != nil || c.Removed != nil):
		return errors.New("binary file exposes text line counts")
	case c.Added != nil && *c.Added < 0:
		return errors.New("added line count is negative")
	case c.Removed != nil && *c.Removed < 0:
		return errors.New("removed line count is negative")
	default:
		return nil
	}
}

func (c Change) Stat() string {
	if c.Binary {
		return "binary"
	}
	parts := make([]string, 0, 2)
	if c.Added != nil {
		parts = append(parts, fmt.Sprintf("+%d", *c.Added))
	}
	if c.Removed != nil {
		parts = append(parts, fmt.Sprintf("-%d", *c.Removed))
	}
	return strings.Join(parts, " ")
}

type DiffRequest struct {
	Workspace string
	Path      string
	Mode      DiffMode
	Format    DiffFormat
	Limit     int
}

func (d DiffRequest) Validate() error {
	if strings.TrimSpace(d.Workspace) == "" {
		return errors.New("workspace diff workspace is empty")
	}
	if d.Mode != "" && d.Mode != DiffModeWorktree && d.Mode != DiffModeBase {
		return fmt.Errorf("workspace diff mode %q is invalid", d.Mode)
	}
	if d.Format != "" && d.Format != DiffFormatRows && d.Format != DiffFormatRaw {
		return fmt.Errorf("workspace diff format %q is invalid", d.Format)
	}
	if d.Limit < 0 {
		return errors.New("workspace diff limit is negative")
	}
	if d.Limit > 0 && d.Format != DiffFormatRows {
		return errors.New("workspace diff limit requires structured rows")
	}
	return nil
}

type DiffRowType string

const (
	DiffRowHunk    DiffRowType = "hunk"
	DiffRowContext DiffRowType = "context"
	DiffRowAdded   DiffRowType = "added"
	DiffRowDeleted DiffRowType = "deleted"
)

type DiffRow struct {
	Type      DiffRowType
	Text      string
	LeftLine  int
	RightLine int
	Code      string
}

func (d DiffRow) Validate() error {
	switch d.Type {
	case DiffRowHunk:
		if d.Text == "" || d.Code != "" || d.LeftLine != 0 || d.RightLine != 0 {
			return errors.New("diff hunk row has an invalid shape")
		}
	case DiffRowContext:
		if d.Code == "" || d.Text != "" || d.LeftLine <= 0 || d.RightLine <= 0 {
			return errors.New("diff context row has an invalid shape")
		}
	case DiffRowAdded:
		if d.Code == "" || d.Text != "" || d.LeftLine != 0 || d.RightLine <= 0 {
			return errors.New("diff addition row has an invalid shape")
		}
	case DiffRowDeleted:
		if d.Code == "" || d.Text != "" || d.LeftLine <= 0 || d.RightLine != 0 {
			return errors.New("diff deletion row has an invalid shape")
		}
	default:
		return fmt.Errorf("diff row type %q is invalid", d.Type)
	}
	return nil
}

type FileDiff struct {
	Change
	Rows []DiffRow
}

type Diff struct {
	Files     []FileDiff
	Patch     string
	Truncated bool
}

func (d Diff) Validate() error {
	if d.Patch != "" && len(d.Files) != 0 {
		return errors.New("workspace diff mixes raw and structured representations")
	}
	for index, file := range d.Files {
		if err := file.Validate(); err != nil {
			return fmt.Errorf("file diff %d: %w", index, err)
		}
		if file.Binary && len(file.Rows) != 0 {
			return fmt.Errorf("file diff %d: binary file has text rows", index)
		}
		for rowIndex, row := range file.Rows {
			if err := row.Validate(); err != nil {
				return fmt.Errorf("file diff %d row %d: %w", index, rowIndex, err)
			}
		}
	}
	return nil
}

// Text returns the raw patch when available and otherwise renders the complete
// structured rows without inventing line content.
func (d Diff) Text() string {
	if d.Patch != "" {
		return d.Patch
	}
	var output strings.Builder
	for index, file := range d.Files {
		if index > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "diff -- %s (%s)\n", file.Path, file.Status)
		for _, row := range file.Rows {
			switch row.Type {
			case DiffRowHunk:
				output.WriteString(row.Text)
			case DiffRowAdded:
				output.WriteByte('+')
				output.WriteString(row.Code)
			case DiffRowDeleted:
				output.WriteByte('-')
				output.WriteString(row.Code)
			case DiffRowContext:
				output.WriteByte(' ')
				output.WriteString(row.Code)
			}
			output.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(output.String(), "\n")
}
