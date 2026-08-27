package document_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
)

func TestDocumentValidate(t *testing.T) {
	if err := (*document.Document)(nil).Validate(); err == nil {
		t.Fatal("nil document must fail validation")
	}
	if err := (&document.Document{}).Validate(); err == nil {
		t.Fatal("document without text or media must fail validation")
	}
	if err := (&document.Document{Text: "hello"}).Validate(); err != nil {
		t.Fatalf("text document: %v", err)
	}
}

func TestDocumentClone(t *testing.T) {
	inline, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	original := &document.Document{ID: "doc-1", Media: inline, Metadata: metadata.Map{}}
	if err := original.Metadata.Set("source", "original"); err != nil {
		t.Fatal(err)
	}

	clone := original.Clone()
	clone.Media.Source.Bytes[0] = 9
	clone.Metadata["source"][1] = 'X'

	if original.Media.Source.Bytes[0] != 1 {
		t.Fatal("clone shares media bytes with original")
	}
	if got := string(original.Metadata["source"]); got != `"original"` {
		t.Fatalf("clone shares metadata with original: %s", got)
	}
	if (*document.Document)(nil).Clone() != nil {
		t.Fatal("nil document clone must remain nil")
	}
}

func TestNewDocumentReturnsNilOnInvalidMedia(t *testing.T) {
	doc, err := document.NewDocument("", &media.Media{})
	if err == nil || doc != nil {
		t.Fatalf("NewDocument() = (%#v, %v), want (nil, error)", doc, err)
	}
}

func TestDocumentContainsOnlyDataFields(t *testing.T) {
	typ := reflect.TypeFor[document.Document]()
	want := []string{"ID", "Text", "Media", "Metadata"}
	if typ.NumField() != len(want) {
		t.Fatalf("Document fields = %d, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Fatalf("field %d = %s, want %s", i, got, name)
		}
	}
}

func TestDocumentJSONRoundTrip(t *testing.T) {
	original := document.Document{ID: "doc-1", Text: "hello", Metadata: metadata.Map{}}
	if err := original.Metadata.Set("source", "test"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded document.Document
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	source, ok, err := decoded.Metadata.Decode[string]("source")
	if decoded.ID != original.ID || decoded.Text != original.Text || err != nil || !ok || source != "test" {
		t.Fatalf("round trip = %#v", decoded)
	}
}
