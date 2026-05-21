package pdf_test

import (
	"bytes"
	"testing"

	"github.com/Tangerg/lynx/document-readers/pdf"
)

// PDF parsing correctness is exercised by the upstream
// github.com/ledongthuc/pdf test suite. The tests here cover only the
// thin lynx wrapper: option plumbing, input validation, error paths.
// End-to-end byte-level coverage will land with a real PDF fixture
// under testdata/ in a follow-up.

func TestNewReader_ValidatesInputs(t *testing.T) {
	if _, err := pdf.NewReader(nil, 100); err == nil {
		t.Error("nil src: expected error, got nil")
	}
	if _, err := pdf.NewReader(bytes.NewReader([]byte{}), 0); err == nil {
		t.Error("zero size: expected error, got nil")
	}
	if _, err := pdf.NewReader(bytes.NewReader([]byte{}), -1); err == nil {
		t.Error("negative size: expected error, got nil")
	}
}

func TestNewReader_AcceptsOptions(t *testing.T) {
	// Just verify the option plumbing — no parsing here. Pass an empty
	// reader so the constructor succeeds; Read() failing is fine.
	src := bytes.NewReader([]byte("not really a pdf"))
	if _, err := pdf.NewReader(src, int64(src.Len()),
		pdf.WithPerPage(),
		pdf.WithSourceName("ignored.pdf"),
		pdf.WithPassword("hunter2"),
	); err != nil {
		t.Fatalf("constructor rejected valid options: %v", err)
	}
}
