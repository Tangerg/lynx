package skills

import (
	"strings"
	"testing"

	skillspec "github.com/Tangerg/lynx/skills"
)

func TestDraftRenderEmitsProvenanceMetadata(t *testing.T) {
	draft := Draft{
		Name:          "run-project-tests",
		Description:   "How to run the test suite. Use when asked to run tests.",
		Body:          "Run `go test ./...` from the module root.",
		CreatedBy:     CreatedByAgent,
		SourceSession: "ses_1",
	}
	content, err := draft.Render()
	if err != nil {
		t.Fatal(err)
	}

	// Provenance must land under the frontmatter metadata block the read-only
	// loader parses — not as top-level keys it would silently drop.
	front, body, err := skillspec.Parse([]byte(content))
	if err != nil {
		t.Fatalf("rendered draft does not parse: %v", err)
	}
	if front.Name != draft.Name || front.Description != draft.Description {
		t.Fatalf("frontmatter round-trip mismatch: %+v", front)
	}
	if got := front.Metadata[MetadataCreatedBy]; got != CreatedByAgent {
		t.Errorf("metadata[%q] = %q, want %q", MetadataCreatedBy, got, CreatedByAgent)
	}
	if got := front.Metadata[MetadataSourceSession]; got != "ses_1" {
		t.Errorf("metadata[%q] = %q, want %q", MetadataSourceSession, got, "ses_1")
	}
	if !strings.Contains(body, "go test") {
		t.Errorf("body round-trip lost the instruction: %q", body)
	}
}

func TestDraftRenderOmitsEmptyProvenance(t *testing.T) {
	draft := Draft{
		Name:        "no-provenance",
		Description: "A hand-authored draft carries no provenance.",
		Body:        "do the thing",
	}
	content, err := draft.Render()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(content, "metadata:") {
		t.Fatalf("rendered an empty metadata block:\n%s", content)
	}
	front, _, err := skillspec.Parse([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if len(front.Metadata) != 0 {
		t.Fatalf("expected no metadata, got %v", front.Metadata)
	}
}

func TestDraftRenderIsDeterministic(t *testing.T) {
	draft := Draft{
		Name:          "stable",
		Description:   "deterministic render keeps content-addressing stable",
		Body:          "step one",
		CreatedBy:     CreatedByAgent,
		SourceSession: "ses_9",
	}
	first, err := draft.Render()
	if err != nil {
		t.Fatal(err)
	}
	second, err := draft.Render()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("render not deterministic:\n%q\n%q", first, second)
	}
}
