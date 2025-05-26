package splitter

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/commons/document"
)

func TestTextSplitter(t *testing.T) {
	doc, err := document.
		NewBuilder().
		WithText(content).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	transDocs, _ := NewTextSplitter("\n").
		Transform(context.Background(), []*document.Document{doc})
	for _, transDoc := range transDocs {
		t.Log(transDoc.Text())
	}
}
