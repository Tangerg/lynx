package pdf_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/Tangerg/lynx/documentreaders/pdf"
)

// Read checks ctx before opening the PDF, so a canceled context errors
// out regardless of the (here invalid) input — no fixture needed.
func TestRead_HonorsContextCancellation(t *testing.T) {
	r, err := pdf.NewReader(bytes.NewReader([]byte("%PDF-1.4")), 8, pdf.WithMetadata(map[string]any{"source": "x.pdf"}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Read(ctx); err != context.Canceled {
		t.Fatalf("canceled context: got %v, want context.Canceled", err)
	}
}
