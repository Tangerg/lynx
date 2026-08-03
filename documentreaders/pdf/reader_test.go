package pdf_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	coremetadata "github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/documentreaders/pdf"
)

func TestNewReader_ValidatesInputs(t *testing.T) {
	if _, err := pdf.NewReader(nil, 100, pdf.Config{}); err == nil {
		t.Error("nil src: expected error, got nil")
	}
	if _, err := pdf.NewReader(bytes.NewReader([]byte{}), 0, pdf.Config{}); err == nil {
		t.Error("zero size: expected error, got nil")
	}
	if _, err := pdf.NewReader(bytes.NewReader([]byte{}), -1, pdf.Config{}); err == nil {
		t.Error("negative size: expected error, got nil")
	}
}

func TestNewReaderAcceptsConfig(t *testing.T) {
	// Just verify configuration plumbing — no parsing here. Pass an empty
	// reader so the constructor succeeds; Read() failing is fine.
	src := bytes.NewReader([]byte("not really a pdf"))
	if _, err := pdf.NewReader(src, int64(src.Len()),
		pdf.Config{PerPage: true, SourceName: "ignored.pdf", Password: "hunter2"},
	); err != nil {
		t.Fatalf("constructor rejected valid config: %v", err)
	}
}

func TestReadWholeExtractsRealPDFAndOwnsMetadata(t *testing.T) {
	payload := testPDF(t, "First page", "Second page")
	metadata, err := coremetadata.FromValues(map[string]any{
		"custom":               "kept",
		pdf.MetadataPagesTotal: 99,
		pdf.MetadataSourceName: "wrong.pdf",
	})
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	reader, err := pdf.NewReader(bytes.NewReader(payload), int64(len(payload)), pdf.Config{
		SourceName: "actual.pdf",
		Metadata:   metadata,
	})
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	if err := metadata.Set("custom", "mutated"); err != nil {
		t.Fatalf("mutate input metadata: %v", err)
	}

	docs, err := reader.Read(t.Context())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(docs) != 1 || !strings.Contains(docs[0].Text, "First page") ||
		!strings.Contains(docs[0].Text, "Second page") {
		t.Fatalf("documents = %+v", docs)
	}
	assertMetadata(t, docs[0].Metadata, pdf.MetadataPagesTotal, 2)
	assertMetadata(t, docs[0].Metadata, pdf.MetadataSourceName, "actual.pdf")
	assertMetadata(t, docs[0].Metadata, "custom", "kept")
}

func TestReadPerPageExtractsPageIdentity(t *testing.T) {
	payload := testPDF(t, "Alpha", "Beta")
	reader, err := pdf.NewReader(bytes.NewReader(payload), int64(len(payload)), pdf.Config{PerPage: true})
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	docs, err := reader.Read(t.Context())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(docs) != 2 || !strings.Contains(docs[0].Text, "Alpha") || !strings.Contains(docs[1].Text, "Beta") {
		t.Fatalf("documents = %+v", docs)
	}
	for index, doc := range docs {
		assertMetadata(t, doc.Metadata, pdf.MetadataPageIndex, index+1)
		assertMetadata(t, doc.Metadata, pdf.MetadataPagesTotal, 2)
	}
}

func TestReadHonorsCanceledContextBeforeParsing(t *testing.T) {
	payload := testPDF(t, "Never parsed")
	reader, err := pdf.NewReader(bytes.NewReader(payload), int64(len(payload)), pdf.Config{})
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if docs, err := reader.Read(ctx); docs != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Read = %+v, %v", docs, err)
	}
}

func TestReadMalformedPDFReturnsError(t *testing.T) {
	payload := []byte("not a PDF")
	reader, err := pdf.NewReader(bytes.NewReader(payload), int64(len(payload)), pdf.Config{})
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	if docs, err := reader.Read(t.Context()); docs != nil || err == nil {
		t.Fatalf("Read = %+v, %v", docs, err)
	}
}

func assertMetadata[T comparable](t *testing.T, metadata coremetadata.Map, key string, want T) {
	t.Helper()
	got, found, err := coremetadata.Decode[T](metadata, key)
	if err != nil || !found || got != want {
		t.Fatalf("metadata %q = %v, found=%v, err=%v; want %v", key, got, found, err, want)
	}
}

// testPDF emits a complete PDF 1.4 byte stream with a valid xref table. It is
// intentionally generated in memory: tests exercise the real parser without a
// binary fixture whose offsets become opaque during review.
func testPDF(t *testing.T, pages ...string) []byte {
	t.Helper()
	if len(pages) == 0 {
		t.Fatal("testPDF requires at least one page")
	}
	fontObject := 3 + len(pages)*2
	objects := make([]string, fontObject)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"

	kids := make([]string, len(pages))
	for index, text := range pages {
		pageObject := 3 + index*2
		contentObject := pageObject + 1
		kids[index] = fmt.Sprintf("%d 0 R", pageObject)
		objects[pageObject-1] = fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
			fontObject, contentObject,
		)
		stream := fmt.Sprintf("BT\n/F1 12 Tf\n72 720 Td\n(%s) Tj\nET\n", escapePDFText(text))
		objects[contentObject-1] = fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages))
	objects[fontObject-1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"

	var output bytes.Buffer
	_, _ = output.WriteString("%PDF-1.4\n% generated by lynx test\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		_, _ = fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xrefOffset := output.Len()
	_, _ = fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for object := 1; object < len(offsets); object++ {
		_, _ = fmt.Fprintf(&output, "%010d 00000 n \n", offsets[object])
	}
	_, _ = fmt.Fprintf(&output,
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets), xrefOffset,
	)
	return output.Bytes()
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(text)
}
