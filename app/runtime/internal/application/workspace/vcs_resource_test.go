package workspace

import (
	"context"
	"strings"
	"testing"
)

const (
	counterexampleMaxWorkspaceChanges      = 10_000
	counterexampleMaxWorkspaceDiffRows     = 5_000
	counterexampleMaxWorkspaceRawDiffBytes = 64 << 20
)

type resourceGitReader struct {
	changes []FileChange
	files   []FileDiff
	patch   string
}

func (reader resourceGitReader) Changes(context.Context, string) ([]FileChange, error) {
	return reader.changes, nil
}

func (reader resourceGitReader) StructuredDiff(context.Context, string, string, bool) ([]FileDiff, error) {
	return reader.files, nil
}

func (reader resourceGitReader) RawDiff(context.Context, string, string, bool) (string, error) {
	return reader.patch, nil
}

func TestVCSRejectsUnboundedCompleteChangeCatalog(t *testing.T) {
	changes := make([]FileChange, counterexampleMaxWorkspaceChanges+1)
	for index := range changes {
		changes[index] = FileChange{Path: "changed", Status: FileStatusModified}
	}
	vcs := NewVCS(NewScope("", "", testPaths{}), resourceGitReader{changes: changes})

	if _, err := vcs.Changes(t.Context(), "/repo"); err == nil {
		t.Fatal("Changes accepted an unbounded complete catalog")
	}
}

func TestVCSDiffAppliesDefaultBudgetAtTheFirstFileBoundary(t *testing.T) {
	rows := make([]DiffRow, counterexampleMaxWorkspaceDiffRows+1)
	for index := range rows {
		rows[index] = DiffRow{Type: DiffRowAdded, Code: "line"}
	}
	vcs := NewVCS(NewScope("", "", testPaths{}), resourceGitReader{
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

func TestVCSRejectsUnboundedRawDiffFromDirectPort(t *testing.T) {
	vcs := NewVCS(NewScope("", "", testPaths{}), resourceGitReader{
		patch: strings.Repeat("p", counterexampleMaxWorkspaceRawDiffBytes+1),
	})

	if _, err := vcs.Diff(t.Context(), DiffInput{CWD: "/repo", Raw: true}); err == nil {
		t.Fatal("Diff accepted an unbounded raw patch")
	}
}
