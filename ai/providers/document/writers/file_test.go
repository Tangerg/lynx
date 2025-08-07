package writers

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/content/document"
)

type mockContentFormater struct {
}

func (m *mockContentFormater) Format(document *document.Document, _ document.MetadataMode) string {
	return document.Text() + "  formated"
}

func TestNewFileWriterBuilder(t *testing.T) {
	build, _ := NewFileWriterBuilder().
		WithPath("/Users/tangerg/Desktop/lynx/ai/commons/document/writer/test.txt").
		WithDocumentMarkers().
		WithAppendMode().
		WithFormatter(&mockContentFormater{}).
		Build()

	doc, err := document.
		NewBuilder().
		WithText("test test").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	err = build.Write(context.Background(), []*document.Document{doc})
	t.Log(err)
}
