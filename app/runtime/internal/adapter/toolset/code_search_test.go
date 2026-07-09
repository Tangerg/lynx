package toolset

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

func TestCodeSearchTool_LocalLiteralSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc targetThing() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, err := newCodeSearchTool(dir, nil, sourcegraphConfig{})
	if err != nil {
		t.Fatalf("newCodeSearchTool: %v", err)
	}

	body, err := tool.Call(t.Context(), `{"literal":"targetThing","limit":5}`)
	if err != nil {
		t.Fatalf("code_search: %v", err)
	}
	var out codeSearchResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Local) != 1 {
		t.Fatalf("local matches = %+v, want one match", out.Local)
	}
	if !strings.HasSuffix(out.Local[0].Path, "server.go") || out.Local[0].Line != 3 {
		t.Fatalf("local match = %+v", out.Local[0])
	}
	if len(out.SuggestedRead) != 1 || out.SuggestedRead[0].Source != "literal" {
		t.Fatalf("suggested reads = %+v", out.SuggestedRead)
	}
}

func TestCodeSearchTool_SemanticSearchUsesIndex(t *testing.T) {
	dir := t.TempDir()
	index := &fakeCodeSearchIndex{
		available: true,
		hits: []codebaseindex.Hit{{
			Path:      "internal/auth/middleware.go",
			StartLine: 12,
			EndLine:   18,
			Text:      "func authenticate() {}\n",
			Score:     0.92,
		}},
	}
	tool, err := newCodeSearchTool(dir, index, sourcegraphConfig{})
	if err != nil {
		t.Fatalf("newCodeSearchTool: %v", err)
	}

	body, err := tool.Call(t.Context(), `{"query":"auth middleware","limit":3}`)
	if err != nil {
		t.Fatalf("code_search: %v", err)
	}
	var out codeSearchResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if index.cwd != dir || index.query != "auth middleware" || index.topK != 3 {
		t.Fatalf("index call = cwd:%q query:%q topK:%d", index.cwd, index.query, index.topK)
	}
	if len(out.Semantic) != 1 || out.Semantic[0].Path != "internal/auth/middleware.go" {
		t.Fatalf("semantic hits = %+v", out.Semantic)
	}
	if len(out.SuggestedRead) != 1 || out.SuggestedRead[0].Source != "semantic" {
		t.Fatalf("suggested reads = %+v", out.SuggestedRead)
	}
}

func TestCodeSearchTool_SourcegraphNoteWhenUnconfigured(t *testing.T) {
	tool, err := newCodeSearchTool(t.TempDir(), nil, sourcegraphConfig{})
	if err != nil {
		t.Fatalf("newCodeSearchTool: %v", err)
	}

	body, err := tool.Call(t.Context(), `{"sourcegraph_query":"repo:github.com/acme/repo auth"}`)
	if err != nil {
		t.Fatalf("code_search: %v", err)
	}
	var out codeSearchResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Sourcegraph) != 0 {
		t.Fatalf("sourcegraph = %+v, want none", out.Sourcegraph)
	}
	if len(out.Notes) != 1 || !strings.Contains(out.Notes[0], "no Sourcegraph endpoint") {
		t.Fatalf("notes = %+v", out.Notes)
	}
}

func TestCodeSearchTool_RequiresAtLeastOneSearchInput(t *testing.T) {
	tool, err := newCodeSearchTool(t.TempDir(), nil, sourcegraphConfig{})
	if err != nil {
		t.Fatalf("newCodeSearchTool: %v", err)
	}
	if _, err := tool.Call(t.Context(), `{}`); err == nil {
		t.Fatal("empty search input: want error")
	}
}

type fakeCodeSearchIndex struct {
	available bool
	hits      []codebaseindex.Hit
	err       error
	cwd       string
	query     string
	topK      int
}

func (f *fakeCodeSearchIndex) Available(context.Context) bool { return f.available }

func (f *fakeCodeSearchIndex) Search(_ context.Context, cwd, query string, topK int) ([]codebaseindex.Hit, error) {
	f.cwd = cwd
	f.query = query
	f.topK = topK
	if f.err != nil {
		return nil, f.err
	}
	return f.hits, nil
}
