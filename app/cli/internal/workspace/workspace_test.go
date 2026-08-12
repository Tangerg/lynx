package workspace

import (
	"strings"
	"testing"
)

func TestChangeOwnsItsRenameAndBinaryInvariants(t *testing.T) {
	t.Parallel()
	added, removed := 3, 2
	tests := []struct {
		name   string
		change Change
		want   string
	}{
		{name: "text", change: Change{Path: "main.go", Status: FileStatusModified, Added: &added, Removed: &removed}},
		{name: "rename", change: Change{Path: "new.go", PreviousPath: "old.go", Status: FileStatusRenamed}},
		{name: "rename missing source", change: Change{Path: "new.go", Status: FileStatusRenamed}, want: "previous path"},
		{name: "source on modification", change: Change{Path: "new.go", PreviousPath: "old.go", Status: FileStatusModified}, want: "non-renamed"},
		{name: "binary counts", change: Change{Path: "logo.png", Status: FileStatusModified, Binary: true, Added: &added}, want: "binary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.change.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkspaceOwnsResolvedIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		workspace Workspace
		want      string
	}{
		{name: "available", workspace: Workspace{Path: "/repo/work", ProjectRoot: "/repo", Availability: Available}},
		{name: "missing", workspace: Workspace{Path: "/gone/work", ProjectRoot: "/gone", Availability: Missing}},
		{name: "relative path", workspace: Workspace{Path: "work", ProjectRoot: "/repo", Availability: Available}, want: "not absolute"},
		{name: "empty project root", workspace: Workspace{Path: "/repo", Availability: Available}, want: "project root is empty"},
		{name: "relative project root", workspace: Workspace{Path: "/repo/work", ProjectRoot: "repo", Availability: Available}, want: "project root is not absolute"},
		{name: "unknown availability", workspace: Workspace{Path: "/repo", ProjectRoot: "/repo", Availability: "unknown"}, want: "availability"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.workspace.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestStructuredDiffValidatesAndRendersEveryRow(t *testing.T) {
	t.Parallel()
	diff := Diff{Files: []FileDiff{{
		Change: Change{Path: "main.go", Status: FileStatusModified},
		Rows: []DiffRow{
			{Type: DiffRowHunk, Text: "@@ -1,2 +1,2 @@"},
			{Type: DiffRowContext, LeftLine: 1, RightLine: 1, Code: "package main"},
			{Type: DiffRowDeleted, LeftLine: 2, Code: "var old = true"},
			{Type: DiffRowAdded, RightLine: 2, Code: "var current = true"},
		},
	}}}
	if err := diff.Validate(); err != nil {
		t.Fatal(err)
	}
	want := "diff -- main.go (modified)\n@@ -1,2 +1,2 @@\n package main\n-var old = true\n+var current = true"
	if got := diff.Text(); got != want {
		t.Fatalf("Text = %q, want %q", got, want)
	}

	invalid := diff
	invalid.Files = append([]FileDiff(nil), diff.Files...)
	invalid.Files[0].Rows = append([]DiffRow(nil), diff.Files[0].Rows...)
	invalid.Files[0].Rows[1].LeftLine = 0
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid context row was accepted")
	}
}

func TestReadRequestRefusesAnAmbiguousLineWindow(t *testing.T) {
	t.Parallel()
	request := ReadRequest{Workspace: "/workspace", Path: "main.go", EndLine: 10}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "requires a start") {
		t.Fatalf("Validate = %v", err)
	}
}

func TestFileListingOwnsPathUniqueness(t *testing.T) {
	t.Parallel()
	listing := FileListing{Entries: []FileEntry{
		{Path: "main.go", Name: "main.go", Type: FileEntryFile},
		{Path: "main.go", Name: "main.go", Type: FileEntryFile},
	}}
	if err := listing.Validate(); err == nil || !strings.Contains(err.Error(), "repeats path") {
		t.Fatalf("Validate = %v, want duplicate path error", err)
	}
}
