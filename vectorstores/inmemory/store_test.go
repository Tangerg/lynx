package inmemory_test

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/inmemory"
)

// fakeEmbeddingModel hashes each input text into a deterministic
// 4-dim vector. Texts sharing a common prefix have higher cosine
// similarity — gives the unit tests something realistic to assert on.
type fakeEmbeddingModel struct{}

func (fakeEmbeddingModel) Call(_ context.Context, req *embedding.Request) (*embedding.Response, error) {
	results := make([]*embedding.Result, 0, len(req.Texts))
	for _, text := range req.Texts {
		r, err := embedding.NewResult(vectorFor(text), &embedding.ResultMetadata{})
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return embedding.NewResponse(results, &embedding.ResponseMetadata{Model: "fake"})
}

func (fakeEmbeddingModel) Stream(ctx context.Context, req *embedding.Request) iter.Seq2[*embedding.Response, error] {
	resp, err := fakeEmbeddingModel{}.Call(ctx, req)
	return func(yield func(*embedding.Response, error) bool) { yield(resp, err) }
}

func (fakeEmbeddingModel) DefaultOptions() embedding.Options {
	opts, _ := embedding.NewOptions("fake")
	return *opts
}

func (fakeEmbeddingModel) Metadata() embedding.ModelMetadata {
	return embedding.ModelMetadata{Provider: "fake"}
}

func (fakeEmbeddingModel) Dimensions(_ context.Context) int64 { return 4 }

// vectorFor maps a text to a deterministic 4-dim float vector.
// Designed so that texts sharing a common prefix have higher cosine
// similarity — gives the unit tests something realistic to assert on.
func vectorFor(text string) []float64 {
	v := []float64{0, 0, 0, 0}
	for i, r := range text {
		v[i%4] += float64(r)
	}
	return v
}

func newStore(t *testing.T) *inmemory.Store {
	t.Helper()
	client, err := embedding.NewClient(fakeEmbeddingModel{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	store, err := inmemory.NewStore(&inmemory.StoreConfig{EmbeddingClient: client})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func mustDoc(t *testing.T, id, text string, metadata map[string]any) *document.Document {
	t.Helper()
	if metadata == nil {
		metadata = map[string]any{}
	}
	return &document.Document{ID: id, Text: text, Metadata: metadata}
}

func TestStore_CreateAndRetrieveBasics(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()

	docs := []*document.Document{
		mustDoc(t, "1", "the quick brown fox", map[string]any{"animal": "fox"}),
		mustDoc(t, "2", "the quick brown bat", map[string]any{"animal": "bat"}),
		mustDoc(t, "3", "unrelated text about ships", map[string]any{"animal": "none"}),
	}
	createReq, err := vectorstore.NewCreateRequest(docs)
	if err != nil {
		t.Fatalf("NewCreateRequest: %v", err)
	}
	if err := store.Create(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}

	retrieveReq, err := vectorstore.NewRetrievalRequest("the quick brown fox")
	if err != nil {
		t.Fatalf("NewRetrievalRequest: %v", err)
	}
	retrieveReq.WithTopK(2)
	got, err := store.Retrieve(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	// Identical vector means doc "1" must rank first.
	if got[0].ID != "1" {
		t.Fatalf("top result = %q, want %q", got[0].ID, "1")
	}
}

func TestStore_CreateRejectsEmptyID(t *testing.T) {
	store := newStore(t)
	docs := []*document.Document{{ID: "", Text: "x"}}
	req := &vectorstore.CreateRequest{Documents: docs}
	if err := store.Create(t.Context(), req); err == nil {
		t.Fatal("Create should reject empty ID")
	}
}

func TestStore_RetrieveHonoursTopK(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := make([]*document.Document, 0, 10)
	for i := range 10 {
		docs = append(docs, mustDoc(t, fmt.Sprintf("d%d", i), fmt.Sprintf("text %d", i), nil))
	}
	createReq, _ := vectorstore.NewCreateRequest(docs)
	if err := store.Create(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	retrieveReq, _ := vectorstore.NewRetrievalRequest("text 3")
	retrieveReq.WithTopK(3)
	got, err := store.Retrieve(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}
}

func TestStore_RetrieveAppliesFilter(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "alpha", map[string]any{"category": "a", "year": 2020}),
		mustDoc(t, "2", "alpha bravo", map[string]any{"category": "a", "year": 2024}),
		mustDoc(t, "3", "bravo", map[string]any{"category": "b", "year": 2024}),
	}
	createReq, _ := vectorstore.NewCreateRequest(docs)
	if err := store.Create(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	expr, err := filter.ParseAndAnalyze(`category == 'a' AND year >= 2024`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	retrieveReq, _ := vectorstore.NewRetrievalRequest("alpha bravo")
	retrieveReq.WithFilter(expr).WithTopK(5)
	got, err := store.Retrieve(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].ID != "2" {
		ids := make([]string, 0, len(got))
		for _, d := range got {
			ids = append(ids, d.ID)
		}
		t.Fatalf("got ids=%v, want [\"2\"]", ids)
	}
}

func TestStore_RetrieveLikePattern(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "alpha", map[string]any{"name": "alpha-1"}),
		mustDoc(t, "2", "beta", map[string]any{"name": "beta-2"}),
		mustDoc(t, "3", "alpha-bravo", map[string]any{"name": "alpha-3"}),
	}
	createReq, _ := vectorstore.NewCreateRequest(docs)
	_ = store.Create(ctx, createReq)

	expr, err := filter.ParseAndAnalyze(`name LIKE 'alpha%'`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	retrieveReq, _ := vectorstore.NewRetrievalRequest("alpha")
	retrieveReq.WithFilter(expr).WithTopK(10)
	got, err := store.Retrieve(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, doc := range got {
		name, _ := doc.Metadata["name"].(string)
		if !strings.HasPrefix(name, "alpha") {
			t.Errorf("doc %q has name=%q, want alpha-prefix", doc.ID, name)
		}
	}
}

func TestStore_Delete(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "x", map[string]any{"keep": true}),
		mustDoc(t, "2", "y", map[string]any{"keep": false}),
		mustDoc(t, "3", "z", map[string]any{"keep": false}),
	}
	createReq, _ := vectorstore.NewCreateRequest(docs)
	_ = store.Create(ctx, createReq)

	expr, err := filter.ParseAndAnalyze(`keep == false`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	delReq, _ := vectorstore.NewDeleteRequest(expr)
	if err := store.Delete(ctx, delReq); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := store.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1", got)
	}
}

func TestStore_RetrieveMinScoreFilters(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "alpha", nil),
		mustDoc(t, "2", "totally unrelated zzzz", nil),
	}
	createReq, _ := vectorstore.NewCreateRequest(docs)
	_ = store.Create(ctx, createReq)

	// Get baseline scores via low threshold to find a discriminating
	// cutoff that mirrors how real callers tune MinScore.
	baseline, err := vectorstore.NewRetrievalRequest("alpha")
	if err != nil {
		t.Fatalf("NewRetrievalRequest: %v", err)
	}
	baseline.WithTopK(10).WithMinScore(0.0)
	all, err := store.Retrieve(ctx, baseline)
	if err != nil {
		t.Fatalf("Retrieve baseline: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("baseline got %d, want 2", len(all))
	}

	// Pick a threshold above the unrelated doc's score but below the
	// exact match's. The fake embedder makes the exact match's score
	// strictly higher than the unrelated doc's.
	allScores := make([]float64, 0, len(all))
	for _, d := range all {
		allScores = append(allScores, inmemory.CosineSimilarity(
			vectorFor("alpha"), vectorFor(d.Text)))
	}
	if allScores[0] <= allScores[1] {
		t.Fatalf("expected exact match to outscore unrelated, got %v", allScores)
	}
	threshold := (allScores[0] + allScores[1]) / 2

	tight, _ := vectorstore.NewRetrievalRequest("alpha")
	tight.WithMinScore(threshold).WithTopK(10)
	got, err := store.Retrieve(ctx, tight)
	if err != nil {
		t.Fatalf("Retrieve tight: %v", err)
	}
	if len(got) != 1 || got[0].ID != "1" {
		ids := make([]string, 0, len(got))
		for _, d := range got {
			ids = append(ids, d.ID)
		}
		t.Fatalf("got ids=%v, want only doc 1", ids)
	}
}

func TestStore_RejectsBadConfig(t *testing.T) {
	if _, err := inmemory.NewStore(nil); err == nil {
		t.Fatal("nil cfg must error")
	}
	if _, err := inmemory.NewStore(&inmemory.StoreConfig{}); err == nil {
		t.Fatal("missing embedding client must error")
	}
}

func TestStore_RetrieveOnEmpty(t *testing.T) {
	store := newStore(t)
	retrieveReq, _ := vectorstore.NewRetrievalRequest("anything")
	got, err := store.Retrieve(t.Context(), retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0", len(got))
	}
}

func TestStore_CreateUpsertsExistingID(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()

	first := []*document.Document{mustDoc(t, "1", "original", map[string]any{"v": 1})}
	createReq, _ := vectorstore.NewCreateRequest(first)
	_ = store.Create(ctx, createReq)

	second := []*document.Document{mustDoc(t, "1", "updated", map[string]any{"v": 2})}
	updateReq, _ := vectorstore.NewCreateRequest(second)
	if err := store.Create(ctx, updateReq); err != nil {
		t.Fatalf("Create upsert: %v", err)
	}

	if got := store.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1 (upsert)", got)
	}
}

func TestSimilarity_CosineSelfIsOne(t *testing.T) {
	v := []float64{1, 2, 3, 4}
	if got := inmemory.CosineSimilarity(v, v); got < 0.999 {
		t.Fatalf("CosineSimilarity(v, v) = %v, want ≈1", got)
	}
}

func TestSimilarity_EuclideanSelfIsOne(t *testing.T) {
	v := []float64{1, 2, 3}
	if got := inmemory.EuclideanSimilarity(v, v); got < 0.999 {
		t.Fatalf("EuclideanSimilarity(v, v) = %v, want 1", got)
	}
}
