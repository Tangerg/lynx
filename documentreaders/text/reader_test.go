package text_test

import (
	"context"
	"errors"
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

func TestReaderEmptySourceReturnsNoDocuments(t *testing.T) {
	reader, err := text.New(strings.NewReader(" \n\t"))
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
	reader, err := text.New(strings.NewReader("never read"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reader.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read error = %v, want context.Canceled", err)
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
