package document_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/document/id"
)

func TestDocument_EnsureID_AssignsWhenEmpty(t *testing.T) {
	doc, _ := document.NewDocument("hello", nil)
	gen := id.NewSha256Generator(nil)

	if err := doc.EnsureID(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	if doc.ID == "" {
		t.Fatal("EnsureID must assign an id")
	}

	// Deterministic generator + same content => stable id, and a second
	// call must not overwrite the existing id.
	first := doc.ID
	if err := doc.EnsureID(context.Background(), id.NewUUIDGenerator()); err != nil {
		t.Fatal(err)
	}
	if doc.ID != first {
		t.Fatalf("EnsureID overwrote existing id: %q -> %q", first, doc.ID)
	}
}

func TestDocument_EnsureID_NilGeneratorIsNoOp(t *testing.T) {
	doc, _ := document.NewDocument("hello", nil)
	if err := doc.EnsureID(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if doc.ID != "" {
		t.Fatal("nil generator must leave id empty")
	}
}

func TestIDAssigner_FillsEmptyOnly(t *testing.T) {
	assigner, err := document.NewIDAssigner(document.IDAssignerConfig{
		Generator: id.NewUUIDGenerator(),
	})
	if err != nil {
		t.Fatal(err)
	}

	withID, _ := document.NewDocument("a", nil)
	withID.ID = "keep-me"
	withoutID, _ := document.NewDocument("b", nil)

	out, err := assigner.Transform(context.Background(), []*document.Document{withID, withoutID})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].ID != "keep-me" {
		t.Fatalf("existing id must be preserved, got %q", out[0].ID)
	}
	if out[1].ID == "" {
		t.Fatal("empty id must be assigned")
	}
}

func TestIDAssigner_Overwrite(t *testing.T) {
	assigner, err := document.NewIDAssigner(document.IDAssignerConfig{
		Generator: id.NewUUIDGenerator(),
		Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := document.NewDocument("a", nil)
	doc.ID = "old"
	out, _ := assigner.Transform(context.Background(), []*document.Document{doc})
	if out[0].ID == "old" || out[0].ID == "" {
		t.Fatalf("Overwrite must replace id, got %q", out[0].ID)
	}
}

func TestIDAssigner_RequiresGenerator(t *testing.T) {
	if _, err := document.NewIDAssigner(document.IDAssignerConfig{}); err == nil {
		t.Fatal("missing generator must error")
	}
}
