package workspace

import "testing"

func TestDiffRequestRequiresStructuredFormatForRowLimit(t *testing.T) {
	t.Parallel()

	request := DiffRequest{Workspace: "/workspace", Mode: DiffModeWorktree, Format: DiffFormatRaw, Limit: 10}
	if err := request.Validate(); err == nil {
		t.Fatal("raw diff accepted a structured row limit")
	}
	request.Format = DiffFormatRows
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
}
