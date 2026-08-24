package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type resourceGitReader struct {
	changes   []FileChange
	files     []FileDiff
	patch     string
	changeMax int
	diffFiles int
	diffRows  int
}

func (reader *resourceGitReader) Changes(_ context.Context, _ string, maxChanges int) ([]FileChange, error) {
	reader.changeMax = maxChanges
	return reader.changes, nil
}

func (reader *resourceGitReader) StructuredDiff(_ context.Context, _, _ string, _ bool, maxFiles, maxRows int) (StructuredDiffResult, error) {
	reader.diffFiles, reader.diffRows = maxFiles, maxRows
	return StructuredDiffResult{Files: reader.files}, nil
}

func (reader *resourceGitReader) RawDiff(context.Context, string, string, bool) (string, error) {
	return reader.patch, nil
}

func TestVCSRejectsUnboundedCompleteChangeCatalog(t *testing.T) {
	changes := make([]FileChange, MaxWorkspaceChanges+1)
	for index := range changes {
		changes[index] = FileChange{Path: "changed", Status: FileStatusModified}
	}
	vcs := NewVCS(NewScope("", "", testPaths{}), &resourceGitReader{changes: changes})

	if _, err := vcs.Changes(t.Context(), "/repo"); !errors.Is(err, ErrVCSResultTooLarge) {
		t.Fatalf("Changes error = %v, want ErrVCSResultTooLarge", err)
	}
}

func TestVCSDiffAppliesDefaultBudgetAtTheFirstFileBoundary(t *testing.T) {
	rows := make([]DiffRow, MaxWorkspaceDiffRows+1)
	for index := range rows {
		rows[index] = DiffRow{Type: DiffRowAdded, Code: "line"}
	}
	vcs := NewVCS(NewScope("", "", testPaths{}), &resourceGitReader{
		files: []FileDiff{{Path: "large.txt", Status: FileStatusModified, Rows: rows}},
	})

	diff, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 0 || !diff.Truncated {
		t.Fatalf("Diff = %d files, truncated=%v; want zero whole files and an honest cut", len(diff.Files), diff.Truncated)
	}
}

func TestVCSDiffAppliesMaterialBudgetAtTheFirstFileBoundary(t *testing.T) {
	vcs := NewVCS(NewScope("", "", testPaths{}), &resourceGitReader{
		files: []FileDiff{{
			Path:   "large.txt",
			Status: FileStatusModified,
			Rows: []DiffRow{{
				Type: DiffRowAdded,
				Code: strings.Repeat("x", MaxWorkspaceRawDiffBytes+1),
			}},
		}},
	})

	diff, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Files) != 0 || !diff.Truncated {
		t.Fatalf("Diff = %d files, truncated=%v; want zero whole files and an honest material cut", len(diff.Files), diff.Truncated)
	}
}

func TestVCSRejectsUnboundedRawDiffFromDirectPort(t *testing.T) {
	vcs := NewVCS(NewScope("", "", testPaths{}), &resourceGitReader{
		patch: strings.Repeat("p", MaxWorkspaceRawDiffBytes+1),
	})

	if _, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo", Raw: true}); !errors.Is(err, ErrVCSResultTooLarge) {
		t.Fatalf("Diff error = %v, want ErrVCSResultTooLarge", err)
	}
}

func TestVCSPassesApplicationLimitsToTheGitReader(t *testing.T) {
	reader := &resourceGitReader{}
	vcs := NewVCS(NewScope("", "", testPaths{}), reader)
	if _, err := vcs.Changes(t.Context(), "/repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo", Limit: 42}); err != nil {
		t.Fatal(err)
	}
	if reader.changeMax != MaxWorkspaceChanges || reader.diffFiles != MaxWorkspaceDiffFiles || reader.diffRows != 42 {
		t.Fatalf(
			"reader limits = changes:%d files:%d rows:%d",
			reader.changeMax,
			reader.diffFiles,
			reader.diffRows,
		)
	}
}
