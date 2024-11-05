package splitter

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/core/document"
)

func TestTextSplitter(t *testing.T) {
	doc := document.
		NewBuilder().
		WithContent(content).
		Build()
	transDocs, _ := NewTextSplitter(nil).
		Transform(context.Background(), []*document.Document{doc})
	for _, transDoc := range transDocs {
		t.Log(transDoc.Content())
	}
}
