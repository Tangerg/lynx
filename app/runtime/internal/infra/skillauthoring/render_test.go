package skillauthoring

import (
	"strings"
	"testing"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

func TestRenderDraftEmitsProvenanceMetadata(t *testing.T) {
	draft := skills.Draft{
		Name:          "run-project-tests",
		Description:   "How to run the test suite. Use when asked to run tests.",
		Body:          "Run `go test ./...` from the module root.",
		CreatedBy:     skills.CreatedByAgent,
		SourceSession: "ses_1",
	}
	content, err := renderDraft(draft)
	if err != nil {
		t.Fatal(err)
	}

	front, body, err := skillspec.Parse(content)
	if err != nil {
		t.Fatalf("rendered draft does not parse: %v", err)
	}
	if front.Name != draft.Name || front.Description != draft.Description {
		t.Fatalf("frontmatter round-trip mismatch: %+v", front)
	}
	if got := front.Metadata[metadataCreatedBy]; got != skills.CreatedByAgent {
		t.Errorf("metadata[%q] = %q, want %q", metadataCreatedBy, got, skills.CreatedByAgent)
	}
	if got := front.Metadata[metadataSourceSession]; got != "ses_1" {
		t.Errorf("metadata[%q] = %q, want %q", metadataSourceSession, got, "ses_1")
	}
	if !strings.Contains(body, "go test") {
		t.Errorf("body round-trip lost the instruction: %q", body)
	}
}

func TestRenderDraftEmitsRevisesMarker(t *testing.T) {
	content, err := renderDraft(skills.Draft{
		Name:        "run-project-tests",
		Description: "A revised version of an existing skill.",
		Body:        "Run `go test ./...` from the module root.",
		CreatedBy:   skills.CreatedByAgent,
		Revises:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	front, _, err := skillspec.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if front.Metadata[metadataRevises] != metadataTrue {
		t.Fatalf("metadata[%q] = %q, want %q", metadataRevises, front.Metadata[metadataRevises], metadataTrue)
	}
}

func TestRenderDraftOmitsEmptyProvenance(t *testing.T) {
	content, err := renderDraft(skills.Draft{
		Name:        "no-provenance",
		Description: "A hand-authored draft carries no provenance.",
		Body:        "do the thing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "metadata:") {
		t.Fatalf("rendered an empty metadata block:\n%s", content)
	}
	front, _, err := skillspec.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(front.Metadata) != 0 {
		t.Fatalf("expected no metadata, got %v", front.Metadata)
	}
}

func TestRenderDraftIsDeterministic(t *testing.T) {
	draft := skills.Draft{
		Name:          "stable",
		Description:   "deterministic render keeps content-addressing stable",
		Body:          "step one",
		CreatedBy:     skills.CreatedByAgent,
		SourceSession: "ses_9",
	}
	first, err := renderDraft(draft)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderDraft(draft)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("render not deterministic:\n%q\n%q", first, second)
	}
}
