package writers

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/content/document"
)

func TestNewFileWriterBuilder(t *testing.T) {
	writer := &FileWriter{
		Path:                "/Users/tangerg/Desktop/lynx/ai/commons/document/writer/test.txt",
		WithDocumentMarkers: true,
		AppendMode:          true,
	}

	doc, err := document.
		NewBuilder().
		WithText("test test").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	err = writer.Write(context.Background(), []*document.Document{doc})
	t.Log(err)
}
