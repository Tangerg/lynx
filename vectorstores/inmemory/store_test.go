package inmemory_test

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	coremetadata "github.com/Tangerg/lynx/core/metadata"
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
	store, err := inmemory.NewStore(inmemory.StoreConfig{EmbeddingClient: client})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func mustDoc(t *testing.T, id, text string, metadata map[string]any) *document.Document {
	t.Helper()
	encoded, err := coremetadata.FromValues(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return &document.Document{ID: id, Text: text, Metadata: encoded}
}

func TestStore_CreateAndRetrieveBasics(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()

	docs := []*document.Document{
		mustDoc(t, "1", "the quick brown fox", map[string]any{"animal": "fox"}),
		mustDoc(t, "2", "the quick brown bat", map[string]any{"animal": "bat"}),
		mustDoc(t, "3", "unrelated text about ships", map[string]any{"animal": "none"}),
	}
	createReq := docs
	err := store.Add(ctx, createReq)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("Len = %d, want 3", got)
	}

	retrieveReq := vectorstore.SearchRequest{Query: "the quick brown fox", TopK: vectorstore.DefaultTopK}
	retrieveReq.TopK = 2
	got, err := store.Search(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	// Identical vector means doc "1" must rank first.
	if got[0].Document.ID != "1" {
		t.Fatalf("top result = %q, want %q", got[0].Document.ID, "1")
	}
}

func TestStore_CreateRejectsEmptyID(t *testing.T) {
	store := newStore(t)
	docs := []*document.Document{{ID: "", Text: "x"}}
	req := docs
	if err := store.Add(t.Context(), req); err == nil {
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
	createReq := docs
	if err := store.Add(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	retrieveReq := vectorstore.SearchRequest{Query: "text 3", TopK: vectorstore.DefaultTopK}
	retrieveReq.TopK = 3
	got, err := store.Search(ctx, retrieveReq)
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
	createReq := docs
	if err := store.Add(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	expr, err := filter.ParseAndAnalyze(`category == 'a' AND year >= 2024`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	retrieveReq := vectorstore.SearchRequest{Query: "alpha bravo", TopK: vectorstore.DefaultTopK}
	retrieveReq.Filter = expr
	retrieveReq.TopK = 5
	got, err := store.Search(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].Document.ID != "2" {
		ids := make([]string, 0, len(got))
		for _, d := range got {
			ids = append(ids, d.Document.ID)
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
	createReq := docs
	_ = store.Add(ctx, createReq)

	expr, err := filter.ParseAndAnalyze(`name LIKE 'alpha%'`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	retrieveReq := vectorstore.SearchRequest{Query: "alpha", TopK: vectorstore.DefaultTopK}
	retrieveReq.Filter = expr
	retrieveReq.TopK = 10
	got, err := store.Search(ctx, retrieveReq)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for _, match := range got {
		name, _, _ := coremetadata.Decode[string](match.Document.Metadata, "name")
		if !strings.HasPrefix(name, "alpha") {
			t.Errorf("doc %q has name=%q, want alpha-prefix", match.Document.ID, name)
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
	createReq := docs
	_ = store.Add(ctx, createReq)

	expr, err := filter.ParseAndAnalyze(`keep == false`)
	if err != nil {
		t.Fatalf("filter.ParseAndAnalyze: %v", err)
	}
	delReq := expr
	if err := store.DeleteWhere(ctx, delReq); err != nil {
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
	createReq := docs
	_ = store.Add(ctx, createReq)

	// Get baseline scores via low threshold to find a discriminating
	// cutoff that mirrors how real callers tune MinScore.
	baseline := vectorstore.SearchRequest{Query: "alpha", TopK: vectorstore.DefaultTopK}
	baseline.TopK = 10
	baseline.MinScore = 0.0
	all, err := store.Search(ctx, baseline)
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
	for _, match := range all {
		allScores = append(allScores, inmemory.CosineSimilarity(
			vectorFor("alpha"), vectorFor(match.Document.Text)))
	}
	if allScores[0] <= allScores[1] {
		t.Fatalf("expected exact match to outscore unrelated, got %v", allScores)
	}
	threshold := (allScores[0] + allScores[1]) / 2

	tight := vectorstore.SearchRequest{Query: "alpha", TopK: vectorstore.DefaultTopK}
	tight.MinScore = threshold
	tight.TopK = 10
	got, err := store.Search(ctx, tight)
	if err != nil {
		t.Fatalf("Retrieve tight: %v", err)
	}
	if len(got) != 1 || got[0].Document.ID != "1" {
		ids := make([]string, 0, len(got))
		for _, d := range got {
			ids = append(ids, d.Document.ID)
		}
		t.Fatalf("got ids=%v, want only doc 1", ids)
	}
}

func TestStore_RejectsBadConfig(t *testing.T) {
	if _, err := inmemory.NewStore(inmemory.StoreConfig{}); err == nil {
		t.Fatal("nil cfg must error")
	}
	if _, err := inmemory.NewStore(inmemory.StoreConfig{}); err == nil {
		t.Fatal("missing embedding client must error")
	}
}

func TestStore_RetrieveOnEmpty(t *testing.T) {
	store := newStore(t)
	retrieveReq := vectorstore.SearchRequest{Query: "anything", TopK: vectorstore.DefaultTopK}
	got, err := store.Search(t.Context(), retrieveReq)
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
	createReq := first
	_ = store.Add(ctx, createReq)

	second := []*document.Document{mustDoc(t, "1", "updated", map[string]any{"v": 2})}
	updateReq := second
	if err := store.Add(ctx, updateReq); err != nil {
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

func TestStore_RetrieveIsNull(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "alpha", map[string]any{"category": "a"}),   // no "owner"
		mustDoc(t, "2", "bravo", map[string]any{"owner": "alice"}),  // has "owner"
		mustDoc(t, "3", "charlie", map[string]any{"category": "c"}), // no "owner"
	}
	createReq := docs
	if err := store.Add(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// IS NULL: matches docs missing "owner".
	expr, err := filter.ParseAndAnalyze(`owner is null`)
	if err != nil {
		t.Fatalf("ParseAndAnalyze(is null): %v", err)
	}
	req := vectorstore.SearchRequest{Query: "x", TopK: vectorstore.DefaultTopK}
	req.Filter = expr
	req.TopK = 10
	got, err := store.Search(ctx, req)
	if err != nil {
		t.Fatalf("Retrieve(is null): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("is null matched %d docs, want 2 (ids 1,3)", len(got))
	}

	// IS NOT NULL: matches the doc that has "owner".
	expr2, err := filter.ParseAndAnalyze(`owner is not null`)
	if err != nil {
		t.Fatalf("ParseAndAnalyze(is not null): %v", err)
	}
	req2 := vectorstore.SearchRequest{Query: "x", TopK: vectorstore.DefaultTopK}
	req2.Filter = expr2
	req2.TopK = 10
	got2, err := store.Search(ctx, req2)
	if err != nil {
		t.Fatalf("Retrieve(is not null): %v", err)
	}
	if len(got2) != 1 || got2[0].Document.ID != "2" {
		t.Fatalf("is not null = %+v, want [id 2]", got2)
	}
}

func TestStore_RetrieveNotIn(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "alpha", map[string]any{"category": "a"}),
		mustDoc(t, "2", "bravo", map[string]any{"category": "b"}),
		mustDoc(t, "3", "charlie", map[string]any{"category": "c"}),
	}
	createReq := docs
	if err := store.Add(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	expr, err := filter.ParseAndAnalyze(`category not in ('a', 'b')`)
	if err != nil {
		t.Fatalf("ParseAndAnalyze: %v", err)
	}
	req := vectorstore.SearchRequest{Query: "x", TopK: vectorstore.DefaultTopK}
	req.Filter = expr
	req.TopK = 10
	got, err := store.Search(ctx, req)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].Document.ID != "3" {
		ids := make([]string, 0, len(got))
		for _, d := range got {
			ids = append(ids, d.Document.ID)
		}
		t.Fatalf("not in ('a','b') matched %v, want [3]", ids)
	}
}

func TestStore_DeleteIDs(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	docs := []*document.Document{
		mustDoc(t, "1", "a", map[string]any{"k": "v"}),
		mustDoc(t, "2", "b", map[string]any{"k": "v"}),
		mustDoc(t, "3", "c", map[string]any{"k": "v"}),
	}
	createReq := docs
	if err := store.Add(ctx, createReq); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Empty slice is a no-op; an unknown id is ignored.
	if err := store.DeleteIDs(ctx, nil); err != nil {
		t.Fatalf("DeleteIDs(nil): %v", err)
	}
	if err := store.DeleteIDs(ctx, []string{"1", "missing"}); err != nil {
		t.Fatalf("DeleteIDs: %v", err)
	}

	req := vectorstore.SearchRequest{Query: "x", TopK: vectorstore.DefaultTopK}
	req.TopK = 10
	got, err := store.Search(ctx, req)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("after DeleteIDs([1]), have %d docs, want 2 (2,3)", len(got))
	}
	for _, match := range got {
		if match.Document.ID == "1" {
			t.Fatal("id 1 should have been deleted")
		}
	}
}
