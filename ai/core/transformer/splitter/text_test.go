package splitter

import (
	"context"
	"github.com/Tangerg/lynx/ai/core/document"
	"testing"
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
