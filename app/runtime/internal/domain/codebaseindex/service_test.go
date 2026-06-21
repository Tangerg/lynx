package codebaseindex

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeEmbedder is a deterministic bag-of-words embedder: each word bumps a hash
// bucket, so cosine similarity tracks word overlap — enough to exercise the
// build→search pipeline without a real embedding API.
type fakeEmbedder struct{}

const fakeDim = 128

func (fakeEmbedder) ID() string { return "fake:v1" }

func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, fakeDim)
		for w := range strings.FieldsSeq(strings.ToLower(t)) {
			v[wordBucket(w)] += 1
		}
		out[i] = v
	}
	return out, nil
}

func wordBucket(w string) int {
	h := 0
	for _, c := range w {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h % fakeDim
}

// memStore is an in-memory codebaseindex.Store for the test.
type memStore struct {
	mu     sync.Mutex
	meta   map[string]Meta
	hashes map[string]map[string]string // cwd → path → hash
	chunks map[string][]Chunk           // cwd → chunks
}

func newMemStore() *memStore {
	return &memStore{meta: map[string]Meta{}, hashes: map[string]map[string]string{}, chunks: map[string][]Chunk{}}
}

func (m *memStore) Meta(_ context.Context, cwd string) (Meta, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.meta[cwd]
	return meta, ok, nil
}
func (m *memStore) SetMeta(_ context.Context, meta Meta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.meta[meta.Cwd] = meta
	return nil
}
func (m *memStore) FileHashes(_ context.Context, cwd string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	maps.Copy(out, m.hashes[cwd])
	return out, nil
}
func (m *memStore) ReplaceFile(_ context.Context, cwd, path, hash string, chunks []Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.chunks[cwd][:0:0]
	for _, c := range m.chunks[cwd] {
		if c.Path != path {
			kept = append(kept, c)
		}
	}
	m.chunks[cwd] = append(kept, chunks...)
	if m.hashes[cwd] == nil {
		m.hashes[cwd] = map[string]string{}
	}
	m.hashes[cwd][path] = hash
	return nil
}
func (m *memStore) DeleteFile(_ context.Context, cwd, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.chunks[cwd][:0:0]
	for _, c := range m.chunks[cwd] {
		if c.Path != path {
			kept = append(kept, c)
		}
	}
	m.chunks[cwd] = kept
	delete(m.hashes[cwd], path)
	return nil
}
func (m *memStore) AllChunks(_ context.Context, cwd string) ([]Chunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Chunk(nil), m.chunks[cwd]...), nil
}
func (m *memStore) Clear(_ context.Context, cwd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.meta, cwd)
	delete(m.hashes, cwd)
	delete(m.chunks, cwd)
	return nil
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSearchFindsRelevantFile: a semantic query surfaces the file whose content
// overlaps it, and Status reports ready.
func TestSearchFindsRelevantFile(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "auth.go", "package auth authenticate login user password credentials session token")
	writeFile(t, cwd, "render.go", "package render paint draw canvas pixel color shader geometry")

	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return fakeEmbedder{}, nil })

	hits, err := ix.Search(context.Background(), cwd, "login user password", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "auth.go" {
		t.Fatalf("top hit = %+v, want auth.go", hits)
	}

	st, _ := ix.Status(context.Background(), cwd)
	if st.State != StateReady || st.ChunkCount == 0 {
		t.Errorf("status = %+v, want ready with chunks", st)
	}
}

// TestNoEmbeddingModel: with no embedder, search reports ErrNoEmbeddingModel and
// Available is false.
func TestNoEmbeddingModel(t *testing.T) {
	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return nil, ErrNoEmbeddingModel })
	if ix.Available(context.Background()) {
		t.Error("Available = true with no embedder")
	}
	if _, err := ix.Search(context.Background(), t.TempDir(), "x", 1); err != ErrNoEmbeddingModel {
		t.Errorf("Search err = %v, want ErrNoEmbeddingModel", err)
	}
}

// TestIncrementalReindex: a new file becomes searchable after Reindex, and only
// changed files are re-embedded (a no-op reindex keeps the corpus).
func TestIncrementalReindex(t *testing.T) {
	cwd := t.TempDir()
	writeFile(t, cwd, "a.go", "alpha widget machine lever")
	store := newMemStore()
	ix := New(store, func(context.Context) (Embedder, error) { return fakeEmbedder{}, nil })

	if err := ix.EnsureIndexed(context.Background(), cwd); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	writeFile(t, cwd, "b.go", "bravo gadget engine piston")
	if err := ix.Reindex(context.Background(), cwd); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	hits, err := ix.Search(context.Background(), cwd, "gadget", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "b.go" {
		t.Fatalf("after adding b.go, top hit for 'gadget' = %+v, want b.go", hits)
	}
}
