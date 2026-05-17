package image_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/image"
)

type fakeImageModel struct {
	defaults *image.Options
	lastReq  *image.Request
	respond  func(req *image.Request) (*image.Response, error)
}

func newFakeImageModel(t *testing.T) *fakeImageModel {
	t.Helper()
	o, err := image.NewOptions("dall-e-3")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeImageModel{defaults: o}
}

func (m *fakeImageModel) DefaultOptions() image.Options { return *m.defaults }
func (m *fakeImageModel) Metadata() image.ModelMetadata          { return image.ModelMetadata{Provider: "fake"} }

func (m *fakeImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	img, _ := image.NewImage("https://example.com/img.png", "")
	res, _ := image.NewResult(img, &image.ResultMetadata{})
	resp, _ := image.NewResponse(res, &image.ResponseMetadata{})
	return resp, nil
}

func TestNewOptions_RequiresModel(t *testing.T) {
	if _, err := image.NewOptions(""); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestResponseFormat_Valid(t *testing.T) {
	if !image.ResponseFormatURL.Valid() {
		t.Fatal("URL must be valid")
	}
	if image.ResponseFormat("garbage").Valid() {
		t.Fatal("garbage must be invalid")
	}
}

func TestNewImage_RequiresOneOfURLOrB64(t *testing.T) {
	if _, err := image.NewImage("", ""); err == nil {
		t.Fatal("both empty must error")
	}
	if _, err := image.NewImage("https://x", ""); err != nil {
		t.Fatalf("URL alone should be fine: %v", err)
	}
}

func TestMergeOptions_RejectsNilBase(t *testing.T) {
	if _, err := image.MergeOptions(nil); err == nil {
		t.Fatal("nil base must error")
	}
}

func TestNewClient_RejectsNilModel(t *testing.T) {
	if _, err := image.NewClient(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestClient_GenerateWithPrompt_ReturnsImage(t *testing.T) {
	model := newFakeImageModel(t)
	client, err := image.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}

	img, _, err := client.GenerateWithPrompt("a duck").Call().Image(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if img.URL == "" {
		t.Fatal("URL is empty")
	}
	if model.lastReq.Prompt != "a duck" {
		t.Fatalf("Prompt = %q", model.lastReq.Prompt)
	}
}

func TestClient_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeImageModel(t)
	model.respond = func(*image.Request) (*image.Response, error) { return nil, want }

	client, _ := image.NewClient(model)
	if _, err := client.GenerateWithPrompt("x").Call().Response(context.Background()); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_GenerateWithRequest_CopiesFields(t *testing.T) {
	model := newFakeImageModel(t)
	client, _ := image.NewClient(model)

	src, _ := image.NewRequest("from-src")
	if _, err := client.GenerateWithRequest(src).Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if model.lastReq.Prompt != "from-src" {
		t.Fatalf("Prompt = %q", model.lastReq.Prompt)
	}
}
