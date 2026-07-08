package codebasesearch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

type fakeIndex struct {
	cwd   string
	query string
	limit int
	hits  []codebaseindex.Hit
	err   error
}

func (f *fakeIndex) Search(_ context.Context, cwd, query string, topK int) ([]codebaseindex.Hit, error) {
	f.cwd = cwd
	f.query = query
	f.limit = topK
	return f.hits, f.err
}

func TestNewBuildsTypedToolAndNormalizesRequest(t *testing.T) {
	index := &fakeIndex{hits: []codebaseindex.Hit{{
		Path:      "app/runtime.go",
		StartLine: 10,
		EndLine:   12,
		Text:      "func run() {}",
		Score:     0.91,
	}}}
	tool, err := New(index)
	if err != nil {
		t.Fatal(err)
	}
	def := tool.Definition()
	if def.Name != "codebase_search" {
		t.Fatalf("Name = %q, want codebase_search", def.Name)
	}
	if !strings.Contains(def.InputSchema, `"query"`) {
		t.Fatalf("InputSchema = %s, want query field", def.InputSchema)
	}

	out, err := tool.Call(context.Background(), `{"query":"  runtime loop  "}`)
	if err != nil {
		t.Fatal(err)
	}
	if index.query != "runtime loop" {
		t.Fatalf("query = %q, want trimmed query", index.query)
	}
	if index.limit != defaultLimit {
		t.Fatalf("limit = %d, want default %d", index.limit, defaultLimit)
	}
	if !strings.Contains(out, "app/runtime.go:10-12") {
		t.Fatalf("output = %q, want rendered hit", out)
	}
}

func TestNewRejectsNilIndex(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("nil index must error")
	}
}

func TestToolRequiresQuery(t *testing.T) {
	tool, err := New(&fakeIndex{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), `{"query":"   "}`); err == nil {
		t.Fatal("blank query must error")
	}
}

func TestToolMapsMissingEmbeddingModel(t *testing.T) {
	tool, err := New(&fakeIndex{err: codebaseindex.ErrNoEmbeddingModel})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Call(context.Background(), `{"query":"runtime"}`)
	if err == nil {
		t.Fatal("missing embedding model must error")
	}
	if errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("error = %v, should be model-facing message", err)
	}
	if !strings.Contains(err.Error(), "no embedding model is configured") {
		t.Fatalf("error = %v, want configuration guidance", err)
	}
}
