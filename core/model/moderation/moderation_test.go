package moderation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/moderation"
)

type fakeModerationModel struct {
	defaults *moderation.Options
	lastReq  *moderation.Request
	respond  func(req *moderation.Request) (*moderation.Response, error)
}

func newFakeModerationModel(t *testing.T) *fakeModerationModel {
	t.Helper()
	o, err := moderation.NewOptions("omni-moderation-latest")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeModerationModel{defaults: o}
}

func (m *fakeModerationModel) DefaultOptions() *moderation.Options { return m.defaults }
func (m *fakeModerationModel) Info() moderation.ModelInfo          { return moderation.ModelInfo{Provider: "fake"} }

func (m *fakeModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	mod := &moderation.Moderation{}
	res, _ := moderation.NewResult(mod, &moderation.ResultMetadata{})
	resp, _ := moderation.NewResponse([]*moderation.Result{res}, &moderation.ResponseMetadata{})
	return resp, nil
}

func TestNewOptions_RequiresModel(t *testing.T) {
	if _, err := moderation.NewOptions(""); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestNewRequest_RequiresTexts(t *testing.T) {
	if _, err := moderation.NewRequest(nil); err == nil {
		t.Fatal("empty texts must error")
	}
}

func TestModeration_FlaggedAggregates(t *testing.T) {
	m := &moderation.Moderation{}
	if m.Flagged() {
		t.Fatal("default Moderation must not be flagged")
	}
	m.Hate.Flagged = true
	if !m.Flagged() {
		t.Fatal("Hate.Flagged must propagate to Flagged()")
	}
}

func TestNewClient_RejectsNilRequest(t *testing.T) {
	if _, err := moderation.NewClient(nil); err == nil {
		t.Fatal("nil request must error")
	}
}

func TestClient_ModerateWithText_ReturnsModeration(t *testing.T) {
	model := newFakeModerationModel(t)
	client, err := moderation.NewClientWithModel(model)
	if err != nil {
		t.Fatal(err)
	}

	mod, _, err := client.ModerateWithText("hi").Call().Moderation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mod == nil {
		t.Fatal("Moderation is nil")
	}
	if model.lastReq.Texts[0] != "hi" {
		t.Fatalf("Texts[0] = %q", model.lastReq.Texts[0])
	}
}

func TestClient_Moderations_ReturnsAll(t *testing.T) {
	model := newFakeModerationModel(t)
	model.respond = func(req *moderation.Request) (*moderation.Response, error) {
		results := []*moderation.Result{}
		for range req.Texts {
			r, _ := moderation.NewResult(&moderation.Moderation{}, &moderation.ResultMetadata{})
			results = append(results, r)
		}
		return moderation.NewResponse(results, &moderation.ResponseMetadata{})
	}
	client, _ := moderation.NewClientWithModel(model)

	got, _, err := client.ModerateWithTexts([]string{"a", "b", "c"}).Call().Moderations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
}

func TestClient_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeModerationModel(t)
	model.respond = func(*moderation.Request) (*moderation.Response, error) { return nil, want }

	client, _ := moderation.NewClientWithModel(model)
	if _, err := client.ModerateWithText("x").Call().Response(context.Background()); !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestMergeOptions_RejectsNilBase(t *testing.T) {
	if _, err := moderation.MergeOptions(nil); err == nil {
		t.Fatal("nil base must error")
	}
}
