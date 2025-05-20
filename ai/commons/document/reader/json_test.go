package reader

import (
	"context"
	"strings"
	"testing"
)

func TestJSONReader_Read(t *testing.T) {
	ctx := context.Background()
	docs, err := NewJSONReader(strings.NewReader("[1,2]")).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		t.Log(doc.Text())
	}
	docs, err = NewJSONReader(strings.NewReader("false")).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		t.Log(doc.Text())
	}

	docs, err = NewJSONReader(strings.NewReader(`["string","test"]`)).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		t.Log(doc.Text())
	}

	docs, err = NewJSONReader(strings.NewReader(`{"key":"value"}`)).Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		t.Log(doc.Text())
	}
}
