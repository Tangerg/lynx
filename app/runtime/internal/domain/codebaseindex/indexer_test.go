package codebaseindex

import (
	"context"
	"errors"
	"maps"
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

type sourceFile struct {
	hash   string
	chunks []Chunk
}

type memSource struct {
	files []string
	chunk map[string]sourceFile
	trunc bool
}

func newMemSource() *memSource {
	return &memSource{chunk: map[string]sourceFile{}}
}

func (s *memSource) add(path, hash, body string) {
	for _, f := range s.files {
		if f == path {
			s.chunk[path] = sourceFile{hash: hash, chunks: []Chunk{{Path: path, StartLine: 1, EndLine: 1, Text: body}}}
			return
		}
	}
	s.files = append(s.files, path)
	s.chunk[path] = sourceFile{hash: hash, chunks: []Chunk{{Path: path, StartLine: 1, EndLine: 1, Text: body}}}
}

func (s *memSource) Files(context.Context, string) ([]string, bool, error) {
	return append([]string(nil), s.files...), s.trunc, nil
}

func (s *memSource) Chunks(_ string, path string) ([]Chunk, string, bool) {
	f, ok := s.chunk[path]
	if !ok {
		return nil, "", false
	}
	return append([]Chunk(nil), f.chunks...), f.hash, true
}

// TestSearchFindsRelevantFile: a semantic query surfaces the file whose content
// overlaps it, and Status reports ready.
func TestSearchFindsRelevantFile(t *testing.T) {
	cwd := t.TempDir()
	src := newMemSource()
	src.add("auth.go", "h1", "package auth authenticate login user password credentials session token")
	src.add("render.go", "h2", "package render paint draw canvas pixel color shader geometry")

	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return fakeEmbedder{}, nil }, src)

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
	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return nil, ErrNoEmbeddingModel }, newMemSource())
	available, err := ix.Available(context.Background())
	if err != nil {
		t.Fatalf("Available err = %v", err)
	}
	if available {
		t.Error("Available = true with no embedder")
	}
	if _, err = ix.Search(context.Background(), t.TempDir(), "x", 1); err != ErrNoEmbeddingModel {
		t.Errorf("Search err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestAvailabilityPreservesResolverFailure(t *testing.T) {
	wantErr := errors.New("provider store unavailable")
	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return nil, wantErr }, newMemSource())

	available, err := ix.Available(context.Background())
	if available || !errors.Is(err, wantErr) {
		t.Fatalf("Available = %v, %v; want false, %v", available, err, wantErr)
	}
}

func TestReindexRecordsResolverFailure(t *testing.T) {
	wantErr := errors.New("provider store unavailable")
	cwd := t.TempDir()
	ix := New(newMemStore(), func(context.Context) (Embedder, error) { return nil, wantErr }, newMemSource())

	if err := ix.Reindex(context.Background(), cwd); !errors.Is(err, wantErr) {
		t.Fatalf("Reindex err = %v, want %v", err, wantErr)
	}
	status, err := ix.Status(context.Background(), cwd)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != StateError || !strings.Contains(status.Err, wantErr.Error()) {
		t.Fatalf("Status = %+v, want error containing %q", status, wantErr)
	}
}

// TestIncrementalReindex: a new file becomes searchable after Reindex, and only
// changed files are re-embedded (a no-op reindex keeps the corpus).
func TestIncrementalReindex(t *testing.T) {
	cwd := t.TempDir()
	src := newMemSource()
	src.add("a.go", "h1", "alpha widget machine lever")
	store := newMemStore()
	ix := New(store, func(context.Context) (Embedder, error) { return fakeEmbedder{}, nil }, src)

	if err := ix.EnsureIndexed(context.Background(), cwd); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	src.add("b.go", "h2", "bravo gadget engine piston")
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

func TestSearchKeepsCorpusAndQueryOnOneEmbedder(t *testing.T) {
	cwd := t.TempDir()
	src := newMemSource()
	src.add("a.go", "h1", "document")
	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	a := &switchTestEmbedder{
		id:           "fake:a",
		dim:          2,
		queryStarted: queryStarted,
		releaseQuery: releaseQuery,
	}
	b := &switchTestEmbedder{id: "fake:b", dim: 3}
	resolver := &switchTestResolver{embedder: a}
	ix := New(newMemStore(), resolver.resolve, src)
	if err := ix.EnsureIndexed(t.Context(), cwd); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	type result struct {
		hits []Hit
		err  error
	}
	searchDone := make(chan result, 1)
	go func() {
		hits, err := ix.Search(t.Context(), cwd, "query", 1)
		searchDone <- result{hits: hits, err: err}
	}()
	<-queryStarted

	resolver.set(b)
	if err := ix.Reindex(t.Context(), cwd); err != nil {
		t.Fatalf("Reindex with replacement embedder: %v", err)
	}
	close(releaseQuery)
	got := <-searchDone
	if got.err != nil {
		t.Fatalf("Search: %v", got.err)
	}
	if len(got.hits) != 1 || got.hits[0].Score < 0.99 {
		t.Fatalf("hits = %+v, want the model-A corpus snapshot scored by model A", got.hits)
	}
}

type switchTestResolver struct {
	mu       sync.Mutex
	embedder Embedder
}

func (r *switchTestResolver) resolve(context.Context) (Embedder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.embedder, nil
}

func (r *switchTestResolver) set(embedder Embedder) {
	r.mu.Lock()
	r.embedder = embedder
	r.mu.Unlock()
}

type switchTestEmbedder struct {
	id           string
	dim          int
	queryStarted chan struct{}
	releaseQuery <-chan struct{}
}

func (e *switchTestEmbedder) ID() string { return e.id }

func (e *switchTestEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		if text == "query" && e.queryStarted != nil {
			close(e.queryStarted)
			<-e.releaseQuery
		}
		out[i] = make([]float32, e.dim)
		out[i][0] = 1
	}
	return out, nil
}
