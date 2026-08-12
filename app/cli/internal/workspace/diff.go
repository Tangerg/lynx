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

func (status FileStatus) Valid() bool {
	switch status {
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

func (change Change) Validate() error {
	switch {
	case strings.TrimSpace(change.Path) == "":
		return errors.New("changed file path is empty")
	case !change.Status.Valid():
		return fmt.Errorf("changed file status %q is invalid", change.Status)
	case change.Status == FileStatusRenamed && strings.TrimSpace(change.PreviousPath) == "":
		return errors.New("renamed file has no previous path")
	case change.Status != FileStatusRenamed && change.PreviousPath != "":
		return errors.New("non-renamed file has a previous path")
	case change.Binary && (change.Added != nil || change.Removed != nil):
		return errors.New("binary file exposes text line counts")
	case change.Added != nil && *change.Added < 0:
		return errors.New("added line count is negative")
	case change.Removed != nil && *change.Removed < 0:
		return errors.New("removed line count is negative")
	default:
		return nil
	}
}

func (change Change) Stat() string {
	if change.Binary {
		return "binary"
	}
	parts := make([]string, 0, 2)
	if change.Added != nil {
		parts = append(parts, fmt.Sprintf("+%d", *change.Added))
	}
	if change.Removed != nil {
		parts = append(parts, fmt.Sprintf("-%d", *change.Removed))
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

func (request DiffRequest) Validate() error {
	if strings.TrimSpace(request.Workspace) == "" {
		return errors.New("workspace diff workspace is empty")
	}
	if request.Mode != "" && request.Mode != DiffModeWorktree && request.Mode != DiffModeBase {
		return fmt.Errorf("workspace diff mode %q is invalid", request.Mode)
	}
	if request.Format != "" && request.Format != DiffFormatRows && request.Format != DiffFormatRaw {
		return fmt.Errorf("workspace diff format %q is invalid", request.Format)
	}
	if request.Limit < 0 {
		return errors.New("workspace diff limit is negative")
	}
	if request.Limit > 0 && request.Format != DiffFormatRows {
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

func (row DiffRow) Validate() error {
	switch row.Type {
	case DiffRowHunk:
		if row.Text == "" || row.Code != "" || row.LeftLine != 0 || row.RightLine != 0 {
			return errors.New("diff hunk row has an invalid shape")
		}
	case DiffRowContext:
		if row.Code == "" || row.Text != "" || row.LeftLine <= 0 || row.RightLine <= 0 {
			return errors.New("diff context row has an invalid shape")
		}
	case DiffRowAdded:
		if row.Code == "" || row.Text != "" || row.LeftLine != 0 || row.RightLine <= 0 {
			return errors.New("diff addition row has an invalid shape")
		}
	case DiffRowDeleted:
		if row.Code == "" || row.Text != "" || row.LeftLine <= 0 || row.RightLine != 0 {
			return errors.New("diff deletion row has an invalid shape")
		}
	default:
		return fmt.Errorf("diff row type %q is invalid", row.Type)
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

func (diff Diff) Validate() error {
	if diff.Patch != "" && len(diff.Files) != 0 {
		return errors.New("workspace diff mixes raw and structured representations")
	}
	for index, file := range diff.Files {
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
func (diff Diff) Text() string {
	if diff.Patch != "" {
		return diff.Patch
	}
	var output strings.Builder
	for index, file := range diff.Files {
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
