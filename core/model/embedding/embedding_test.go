package embedding_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
)

// fakeEmbeddingModel is the test mock used across the embedding suite.
// Each Call captures the request so tests can assert what reached the
// model layer.
type fakeEmbeddingModel struct {
	provider    string
	defaultOpts *embedding.Options
	lastReq     *embedding.Request
	respond     func(req *embedding.Request) (*embedding.Response, error)
}

func newFakeEmbeddingModel(t *testing.T) *fakeEmbeddingModel {
	t.Helper()
	defaults, err := embedding.NewOptions("text-embedding-3-small")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeEmbeddingModel{provider: "fake", defaultOpts: defaults}
}

func (m *fakeEmbeddingModel) DefaultOptions() embedding.Options { return *m.defaultOpts }
func (m *fakeEmbeddingModel) Metadata() embedding.ModelMetadata {
	return embedding.ModelMetadata{Provider: m.provider}
}
func (m *fakeEmbeddingModel) Dimensions(_ context.Context) int64 { return 4 }

func (m *fakeEmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	return responseFor(req.Texts), nil
}

func responseFor(texts []string) *embedding.Response {
	results := make([]*embedding.Result, 0, len(texts))
	for i := range texts {
		r, _ := embedding.NewResult([]float64{0.1, 0.2, 0.3, 0.4}, &embedding.ResultMetadata{Index: int64(i), ModalityType: embedding.Text})
		results = append(results, r)
	}
	resp, _ := embedding.NewResponse(results, &embedding.ResponseMetadata{})
	return resp
}

func TestNewOptions_RequiresModel(t *testing.T) {
	if _, err := embedding.NewOptions(""); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestEncodingFormat_Valid(t *testing.T) {
	if !embedding.EncodingFormatFloat.Valid() {
		t.Fatal("float must be valid")
	}
	if embedding.EncodingFormat("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestOptions_CloneDeepCopy(t *testing.T) {
	d := int64(64)
	opts := &embedding.Options{Model: "m", Dimensions: &d, Extra: map[string]any{"k": 1}}

	clone := opts.Clone()
	*clone.Dimensions = 999
	clone.Extra["k"] = 999

	if *opts.Dimensions != 64 || opts.Extra["k"] != 1 {
		t.Fatal("Clone is shallow")
	}
}

func TestMergeOptions(t *testing.T) {
	d := int64(32)
	base := &embedding.Options{Model: "base"}
	override := &embedding.Options{Model: "override", Dimensions: &d, EncodingFormat: embedding.EncodingFormatFloat}

	merged, err := embedding.MergeOptions(base, override, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" {
		t.Fatalf("Model = %q", merged.Model)
	}
	if *merged.Dimensions != 32 {
		t.Fatalf("Dimensions = %d", *merged.Dimensions)
	}

	if _, err := embedding.MergeOptions(nil); err == nil {
		t.Fatal("nil base must error")
	}
}

func TestNewRequest_RequiresTexts(t *testing.T) {
	if _, err := embedding.NewRequest(nil); err == nil {
		t.Fatal("empty texts must error")
	}
}

func TestNewClient_RejectsNilModel(t *testing.T) {
	if _, err := embedding.NewClient(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestClient_EmbedWithText_BuildsSingleEntryRequest(t *testing.T) {
	model := newFakeEmbeddingModel(t)
	client, err := embedding.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}

	v, _, err := client.EmbedWithText("hello").Call().Embedding(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 4 {
		t.Fatalf("len = %d, want 4", len(v))
	}
	if len(model.lastReq.Texts) != 1 {
		t.Fatalf("Texts len = %d, want 1", len(model.lastReq.Texts))
	}
}

func TestClient_Embeddings_ReturnsAll(t *testing.T) {
	model := newFakeEmbeddingModel(t)
	client, _ := embedding.NewClient(model)

	got, _, err := client.EmbedWithTexts([]string{"a", "b", "c"}).Call().Embeddings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d vectors, want 3", len(got))
	}
}

func TestClient_EmbedWithDocument(t *testing.T) {
	model := newFakeEmbeddingModel(t)
	client, _ := embedding.NewClient(model)

	doc := &document.Document{Text: "doc-text"}
	if _, err := client.EmbedWithDocument(doc).Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if model.lastReq.Texts[0] != "doc-text" {
		t.Fatalf("Texts[0] = %q", model.lastReq.Texts[0])
	}
}

func TestClient_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeEmbeddingModel(t)
	model.respond = func(*embedding.Request) (*embedding.Response, error) { return nil, want }

	client, _ := embedding.NewClient(model)
	if _, err := client.EmbedWithText("x").Call().Response(context.Background()); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestClient_MiddlewareApplied(t *testing.T) {
	model := newFakeEmbeddingModel(t)
	client, _ := embedding.NewClient(model)

	calls := 0
	mw := embedding.Middleware(func(next embedding.Handler) embedding.Handler {
		return embedding.HandlerFunc(func(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
			calls++
			return next.Call(ctx, req)
		})
	})

	if _, err := client.EmbedWithText("x").WithMiddlewares(mw).Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("middleware ran %d times, want 1", calls)
	}
}

func TestClient_ChatErrorMessageHasContext(t *testing.T) {
	if _, err := embedding.NewClientRequest(nil); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "embedding.NewClientRequest") {
		t.Fatalf("error %q must include package context", err.Error())
	}
}
