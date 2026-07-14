package documentpipeline_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/documentpipeline"
	"github.com/Tangerg/lynx/documentpipeline/id"
)

func TestIDAssigner_FillsEmptyOnly(t *testing.T) {
	assigner, err := documentpipeline.NewIDAssigner(documentpipeline.IDAssignerConfig{
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
	assigner, err := documentpipeline.NewIDAssigner(documentpipeline.IDAssignerConfig{
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
	if _, err := documentpipeline.NewIDAssigner(documentpipeline.IDAssignerConfig{}); err == nil {
		t.Fatal("missing generator must error")
	}
}
