package text_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/scope/etl"
	"github.com/Tangerg/scope/etl/text"
)

type pointerReader struct{}

func (*pointerReader) Read([]byte) (int, error) { return 0, io.EOF }

func TestReader(t *testing.T) {
	reader, err := text.NewReader(strings.NewReader("hello"), text.ReaderConfig{})
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

func TestReaderEmptySourceReturnsNoDocuments(t *testing.T) {
	reader, err := text.NewReader(strings.NewReader(" \n\t"), text.ReaderConfig{})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := reader.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if docs != nil {
		t.Fatalf("documents = %#v, want nil", docs)
	}
}

func TestReaderHonorsCanceledContext(t *testing.T) {
	reader, err := text.NewReader(strings.NewReader("never read"), text.ReaderConfig{})
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
	reader, err := text.NewReader(strings.NewReader("12345"), text.ReaderConfig{SourceBudget: budget})
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
	if _, err := text.NewReader(nil, text.ReaderConfig{}); err == nil {
		t.Fatal("nil reader must fail")
	}
	if _, err := text.NewReader(typedNil, text.ReaderConfig{}); err == nil {
		t.Fatal("typed nil reader must fail")
	}
}
