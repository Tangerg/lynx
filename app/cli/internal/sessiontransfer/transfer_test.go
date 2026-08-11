package sessiontransfer

import "testing"

func TestDocumentOwnsAndValidatesPortableContent(t *testing.T) {
	body := []byte(`{"version":17}`)
	document, err := NewDocument(JSON, body)
	if err != nil {
		t.Fatal(err)
	}
	body[0] = 'x'
	copy := document.Bytes()
	copy[0] = 'x'
	if got := string(document.Bytes()); got != `{"version":17}` {
		t.Fatalf("document body = %q", got)
	}
	if !document.Importable() {
		t.Fatal("valid JSON document is not importable")
	}
	markdown, err := NewDocument(Markdown, []byte("# Session"))
	if err != nil {
		t.Fatal(err)
	}
	if markdown.Importable() {
		t.Fatal("Markdown document is importable")
	}
}
