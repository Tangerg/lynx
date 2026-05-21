package html_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/document-readers/html"
)

const samplePage = `<!doctype html>
<html>
<head>
  <title>Test Page</title>
  <meta name="description" content="An example page for the HTML reader.">
  <link rel="canonical" href="https://example.com/test">
  <style>.hidden { display: none }</style>
  <script>console.log('drop me');</script>
</head>
<body>
  <h1>Welcome</h1>
  <article>
    <h2>First Article</h2>
    <p>Hello, world.</p>
  </article>
  <article>
    <h2>Second Article</h2>
    <p>Goodbye, world.</p>
  </article>
  <script>alert('still drop me')</script>
</body>
</html>`

func TestWholePage(t *testing.T) {
	r, err := html.NewReader(strings.NewReader(samplePage), html.WithSourceName("test.html"))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("want 1 doc, got %d", len(docs))
	}
	body := docs[0].Text
	if !strings.Contains(body, "Hello, world.") || !strings.Contains(body, "Goodbye, world.") {
		t.Errorf("body missing expected text: %q", body)
	}
	if strings.Contains(body, "drop me") {
		t.Errorf("script content leaked into body: %q", body)
	}
	if got := docs[0].Metadata[html.MetadataTitle]; got != "Test Page" {
		t.Errorf("title: want %q, got %v", "Test Page", got)
	}
	if got := docs[0].Metadata[html.MetadataCanonical]; got != "https://example.com/test" {
		t.Errorf("canonical: got %v", got)
	}
	if got := docs[0].Metadata[html.MetadataSourceName]; got != "test.html" {
		t.Errorf("source: got %v", got)
	}
}

func TestSelector(t *testing.T) {
	r, err := html.NewReader(
		strings.NewReader(samplePage),
		html.WithSelector("article"),
	)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("want 2 articles, got %d", len(docs))
	}
	if !strings.Contains(docs[0].Text, "Hello, world.") {
		t.Errorf("docs[0]: %q", docs[0].Text)
	}
	if !strings.Contains(docs[1].Text, "Goodbye, world.") {
		t.Errorf("docs[1]: %q", docs[1].Text)
	}
	for i, d := range docs {
		if got := d.Metadata[html.MetadataSelector]; got != "article" {
			t.Errorf("docs[%d] selector: got %v", i, got)
		}
	}
}
