package vectorstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
)

func TestNewRetrievalRequest_RejectsEmpty(t *testing.T) {
	if _, err := vectorstore.NewRetrievalRequest(""); err == nil {
		t.Fatal("empty query must error")
	}
}

func TestNewRetrievalRequest_AppliesDefaults(t *testing.T) {
	req, err := vectorstore.NewRetrievalRequest("hi")
	if err != nil {
		t.Fatal(err)
	}
	if req.TopK != vectorstore.DefaultTopK {
		t.Fatalf("TopK = %d, want %d", req.TopK, vectorstore.DefaultTopK)
	}
	if req.MinScore != vectorstore.AcceptAllScores {
		t.Fatalf("MinScore = %f, want %f", req.MinScore, vectorstore.AcceptAllScores)
	}
}

func TestRetrievalRequest_WithTopK_IgnoresNonPositive(t *testing.T) {
	req, _ := vectorstore.NewRetrievalRequest("hi")
	req.WithTopK(20)
	req.WithTopK(-1) // should be ignored
	if req.TopK != 20 {
		t.Fatalf("TopK = %d, want 20", req.TopK)
	}
}

func TestRetrievalRequest_WithMinScore_IgnoresOutOfRange(t *testing.T) {
	req, _ := vectorstore.NewRetrievalRequest("hi")
	req.WithMinScore(0.7)
	req.WithMinScore(1.5)  // out of range
	req.WithMinScore(-0.1) // out of range
	if req.MinScore != 0.7 {
		t.Fatalf("MinScore = %f, want 0.7", req.MinScore)
	}
}

func TestRetrievalRequest_Validate(t *testing.T) {
	cases := []struct {
		name string
		req  *vectorstore.RetrievalRequest
		ok   bool
	}{
		{"nil", nil, false},
		{"empty query", &vectorstore.RetrievalRequest{TopK: 5}, false},
		{"zero topk", &vectorstore.RetrievalRequest{Query: "hi", TopK: 0}, false},
		{"out-of-range minscore", &vectorstore.RetrievalRequest{Query: "hi", TopK: 5, MinScore: 1.5}, false},
		{"valid", &vectorstore.RetrievalRequest{Query: "hi", TopK: 5, MinScore: 0.5}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNewCreateRequest_RejectsEmpty(t *testing.T) {
	if _, err := vectorstore.NewCreateRequest(nil); err == nil {
		t.Fatal("nil docs must error")
	}
	if _, err := vectorstore.NewCreateRequest([]*document.Document{}); err == nil {
		t.Fatal("empty docs must error")
	}
}

func TestNewDeleteRequest_RequiresFilter(t *testing.T) {
	if _, err := vectorstore.NewDeleteRequest(nil); err == nil {
		t.Fatal("nil filter must error")
	}
}

// fakeCreator captures Create calls for the document writer adapter test.
type fakeCreator struct {
	got *vectorstore.CreateRequest
	err error
}

func (c *fakeCreator) Create(_ context.Context, req *vectorstore.CreateRequest) error {
	c.got = req
	return c.err
}

func TestNewDocumentWriter_DelegatesToCreator(t *testing.T) {
	c := &fakeCreator{}
	w := vectorstore.NewDocumentWriter(c)

	doc, _ := document.NewDocument("hi", nil)
	if err := w.Write(context.Background(), []*document.Document{doc}); err != nil {
		t.Fatal(err)
	}
	if c.got == nil || len(c.got.Documents) != 1 {
		t.Fatalf("creator did not receive the docs")
	}
}

func TestNewDocumentWriter_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	c := &fakeCreator{err: want}
	w := vectorstore.NewDocumentWriter(c)

	doc, _ := document.NewDocument("hi", nil)
	if err := w.Write(context.Background(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewDocumentWriter_RejectsEmptyDocs(t *testing.T) {
	c := &fakeCreator{}
	w := vectorstore.NewDocumentWriter(c)

	if err := w.Write(context.Background(), nil); err == nil {
		t.Fatal("empty docs must error")
	}
	if c.got != nil {
		t.Fatal("creator should not be called when validation fails")
	}
}

// Compile-time interface check — sanity asserts the public surface.
var _ ast.Expr = ast.Expr(nil)
