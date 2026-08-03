package text_test

import (
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/documentreaders/text"
)

type pointerReader struct{}

func (*pointerReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestReader(t *testing.T) {
	reader, err := text.New(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := reader.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Text != "hello" {
		t.Fatalf("documents = %#v", docs)
	}
}

func TestNewRejectsNil(t *testing.T) {
	var typedNil *pointerReader
	if _, err := text.New(nil); err == nil {
		t.Fatal("nil reader must fail")
	}
	if _, err := text.New(typedNil); err == nil {
		t.Fatal("typed nil reader must fail")
	}
}
