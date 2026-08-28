package json_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/scope/etl"
	jsonreader "github.com/Tangerg/scope/etl/json"
)

type pointerReader struct{}

func (*pointerReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestReaderArray(t *testing.T) {
	reader, err := jsonreader.NewReader(strings.NewReader(`[{"id":1},{"id":2}]`), jsonreader.ReaderConfig{})
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

func TestReaderSingleValue(t *testing.T) {
	for _, input := range []string{`{"id":1}`, `42`, `"text"`, `null`} {
		reader, err := jsonreader.NewReader(strings.NewReader(input), jsonreader.ReaderConfig{})
		if err != nil {
			t.Fatal(err)
		}
		docs, err := reader.Read(t.Context())
		if err != nil {
			t.Fatalf("Read(%s): %v", input, err)
		}
		if len(docs) != 1 || docs[0].Text != input {
			t.Fatalf("Read(%s) = %#v", input, docs)
		}
	}
}

func TestReaderHonorsCanceledContext(t *testing.T) {
	reader, err := jsonreader.NewReader(strings.NewReader(`{"id":1}`), jsonreader.ReaderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reader.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read error = %v, want context.Canceled", err)
	}
}

func TestReaderRejectsSourceBeyondBudget(t *testing.T) {
	budget, err := etl.NewSourceBudget(4)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := jsonreader.NewReader(strings.NewReader(`{"id":1}`), jsonreader.ReaderConfig{SourceBudget: budget})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := reader.Read(t.Context())
	if docs != nil || !errors.Is(err, etl.ErrSourceTooLarge) {
		t.Fatalf("Read = (%v, %v), want nil ErrSourceTooLarge", docs, err)
	}
}

func TestNewReaderRejectsNil(t *testing.T) {
	var typedNil *pointerReader
	if _, err := jsonreader.NewReader(nil, jsonreader.ReaderConfig{}); err == nil {
		t.Fatal("nil reader must fail")
	}
	if _, err := jsonreader.NewReader(typedNil, jsonreader.ReaderConfig{}); err == nil {
		t.Fatal("typed nil reader must fail")
	}
}

func TestReaderRejectsMalformedArray(t *testing.T) {
	reader, err := jsonreader.NewReader(strings.NewReader(`[{"id":1}`), jsonreader.ReaderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(t.Context()); err == nil {
		t.Fatal("malformed JSON array was accepted")
	}
}
