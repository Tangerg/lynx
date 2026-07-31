package documentreaders_test

import (
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/documentreaders"
)

type pointerReader struct{}

func (*pointerReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestTextReader(t *testing.T) {
	reader, err := documentreaders.NewTextReader(strings.NewReader("hello"))
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

func TestJSONReaderArray(t *testing.T) {
	reader, err := documentreaders.NewJSONReader(strings.NewReader(`[{"id":1},{"id":2}]`))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := reader.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Text != `{"id":1}` || docs[1].Text != `{"id":2}` {
		t.Fatalf("documents = %#v", docs)
	}
}

func TestReadersRejectNil(t *testing.T) {
	var typedNil *pointerReader
	if _, err := documentreaders.NewTextReader(nil); err == nil {
		t.Fatal("nil text reader must fail")
	}
	if _, err := documentreaders.NewJSONReader(nil); err == nil {
		t.Fatal("nil JSON reader must fail")
	}
	if _, err := documentreaders.NewTextReader(typedNil); err == nil {
		t.Fatal("typed nil text reader must fail")
	}
	if _, err := documentreaders.NewJSONReader(typedNil); err == nil {
		t.Fatal("typed nil JSON reader must fail")
	}
}

func TestJSONReaderRejectsMalformedArray(t *testing.T) {
	reader, err := documentreaders.NewJSONReader(strings.NewReader(`[{"id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(t.Context()); err == nil {
		t.Fatal("malformed JSON array was accepted")
	}
}
