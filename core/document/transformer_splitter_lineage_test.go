package document_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/document/id"
)

func TestSplitter_StampsChunkLineage(t *testing.T) {
	splitter, err := document.NewSplitter(document.SplitterConfig{
		SplitFunc: func(_ context.Context, text string) ([]string, error) {
			// Includes an empty chunk to verify it is dropped before
			// chunk_index / chunk_total are computed.
			return []string{"a", "", "b", "c"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	parent, _ := document.NewDocument("ignored", nil)
	parent.ID = "parent-1"
	parent.Score = 0.75
	parent.Metadata["source"] = "manual"

	chunks, err := splitter.Transform(context.Background(), []*document.Document{parent})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 3 {
		t.Fatalf("want 3 non-empty chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if chunk.Metadata[document.MetadataKeyChunkIndex] != i {
			t.Fatalf("chunk %d: chunk_index = %v", i, chunk.Metadata[document.MetadataKeyChunkIndex])
		}
		if chunk.Metadata[document.MetadataKeyChunkTotal] != 3 {
			t.Fatalf("chunk %d: chunk_total = %v", i, chunk.Metadata[document.MetadataKeyChunkTotal])
		}
		if chunk.Metadata[document.MetadataKeyParentID] != "parent-1" {
			t.Fatalf("chunk %d: parent id = %v", i, chunk.Metadata[document.MetadataKeyParentID])
		}
		if chunk.Metadata["source"] != "manual" {
			t.Fatalf("chunk %d: original metadata not carried through", i)
		}
		if chunk.Score != 0.75 {
			t.Fatalf("chunk %d: score = %v, want parent score passthrough", i, chunk.Score)
		}
	}
}

func TestSplitter_NoParentIDWhenSourceUnidentified(t *testing.T) {
	splitter, _ := document.NewSplitter(document.SplitterConfig{
		SplitFunc: func(_ context.Context, _ string) ([]string, error) {
			return []string{"x"}, nil
		},
	})

	parent, _ := document.NewDocument("body", nil) // ID stays ""
	chunks, _ := splitter.Transform(context.Background(), []*document.Document{parent})

	if _, ok := chunks[0].Metadata[document.MetadataKeyParentID]; ok {
		t.Fatal("parent_document_id must be absent when source has no id")
	}
}

func TestSplitter_AssignsChunkIDs(t *testing.T) {
	splitter, _ := document.NewSplitter(document.SplitterConfig{
		IDGenerator: id.NewSha256Generator(nil),
		SplitFunc: func(_ context.Context, _ string) ([]string, error) {
			return []string{"x", "y"}, nil
		},
	})

	parent, _ := document.NewDocument("body", nil)
	chunks, _ := splitter.Transform(context.Background(), []*document.Document{parent})

	if chunks[0].ID == "" || chunks[1].ID == "" {
		t.Fatal("chunks must get ids when IDGenerator is set")
	}
	if chunks[0].ID == chunks[1].ID {
		t.Fatal("distinct chunks must get distinct ids")
	}
}
