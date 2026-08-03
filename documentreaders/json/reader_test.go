package json_test

import (
	"io"
	"strings"
	"testing"

	jsonreader "github.com/Tangerg/lynx/documentreaders/json"
)

type pointerReader struct{}

func (*pointerReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestReaderArray(t *testing.T) {
	reader, err := jsonreader.New(strings.NewReader(`[{"id":1},{"id":2}]`))
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

func TestNewRejectsNil(t *testing.T) {
	var typedNil *pointerReader
	if _, err := jsonreader.New(nil); err == nil {
		t.Fatal("nil reader must fail")
	}
	if _, err := jsonreader.New(typedNil); err == nil {
		t.Fatal("typed nil reader must fail")
	}
}

func TestReaderRejectsMalformedArray(t *testing.T) {
	reader, err := jsonreader.New(strings.NewReader(`[{"id":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(t.Context()); err == nil {
		t.Fatal("malformed JSON array was accepted")
	}
}
