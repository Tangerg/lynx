package documentreaders_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/documentreaders"
)

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
	if _, err := documentreaders.NewTextReader(nil); err == nil {
		t.Fatal("nil text reader must fail")
	}
	if _, err := documentreaders.NewJSONReader(nil); err == nil {
		t.Fatal("nil JSON reader must fail")
	}
}
