package writer

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/core/document"
)

type mockContentFormater struct {
}

func (m *mockContentFormater) Format(document *document.Document, _ document.MetadataMode) string {
	return document.Content() + "  formated"
}

func TestNewFileWriterBuilder(t *testing.T) {
	build, _ := NewFileWriterBuilder().
		WithPath("C:\\Users\\Tangerg\\Desktop\\testfilewriter.txt").
		WithDocumentMarkers().
		WithAppendMode().
		Build()

	doc := document.
		NewBuilder().
		WithContent("test test").
		Build().
		SetContentFormatter(&mockContentFormater{})

	err := build.Write(context.Background(), []*document.Document{doc})
	t.Log(err)
}
