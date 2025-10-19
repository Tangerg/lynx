package writers

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/media/document"
)

func TestNewFileWriterBuilder(t *testing.T) {
	writerConf := &FileWriterConfig{
		Path:                "/Users/tangerg/Desktop/lynx/ai/commons/document/writer/test.txt",
		WithDocumentMarkers: true,
		AppendMode:          true,
	}
	writer, _ := NewFileWriter(writerConf)

	doc, err := document.NewDocument("test test", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = writer.Write(context.Background(), []*document.Document{doc})
	t.Log(err)
}
