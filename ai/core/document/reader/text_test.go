package reader

import (
	"context"
	"strings"
	"testing"
)

func TestNewTextReader(t *testing.T) {
	tr := NewTextReader(strings.NewReader("hello world"))
	docs, err := tr.Read(context.Background())
	if err != nil {
		t.Fatal(err)
		return
	}
	for _, doc := range docs {
		t.Log(doc.Content())
	}
}
