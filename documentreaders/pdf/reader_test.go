package pdf_test

import (
	"bytes"
	"testing"

	"github.com/Tangerg/lynx/documentreaders/pdf"
)

// PDF parsing correctness is exercised by the upstream
// github.com/ledongthuc/pdf test suite. The tests here cover only the
// thin lynx wrapper: configuration, input validation, and error paths.
// End-to-end byte-level coverage will land with a real PDF fixture
// under testdata/ in a follow-up.

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
