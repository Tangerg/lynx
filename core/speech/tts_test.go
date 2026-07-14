package speech_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	tts "github.com/Tangerg/lynx/core/speech"
)

type fakeTTSModel struct {
	defaults    *tts.Options
	lastReq     *tts.Request
	respond     func(req *tts.Request) (*tts.Response, error)
	streamYield []*tts.Response
	streamErr   error
}

func newFakeTTSModel(t *testing.T) *fakeTTSModel {
	t.Helper()
	o, err := tts.NewOptions("tts-1")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeTTSModel{defaults: o}
}

func (m *fakeTTSModel) DefaultOptions() tts.Options { return *m.defaults }
func (m *fakeTTSModel) Metadata() tts.ModelMetadata { return tts.ModelMetadata{Provider: "fake"} }

func (m *fakeTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	res, _ := tts.NewResult([]byte("audio"), &tts.ResultMetadata{})
	return tts.NewResponse(res, &tts.ResponseMetadata{})
}

func (m *fakeTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	m.lastReq = req
	return func(yield func(*tts.Response, error) bool) {
		if m.streamErr != nil {
			yield(nil, m.streamErr)
			return
		}
		for _, resp := range m.streamYield {
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func TestNewOptions_RequiresModel(t *testing.T) {
	if _, err := tts.NewOptions(""); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestNewRequest_RequiresText(t *testing.T) {
	if _, err := tts.NewRequest(""); err == nil {
		t.Fatal("empty text must error")
	}
}

func TestNewResult_RequiresSpeechAndMetadata(t *testing.T) {
	if _, err := tts.NewResult(nil, &tts.ResultMetadata{}); err == nil {
		t.Fatal("nil speech must error")
	}
	if _, err := tts.NewResult([]byte("a"), nil); err == nil {
		t.Fatal("nil metadata must error")
	}
}

func TestClient_SynthesizeWithText_CallReturnsBytes(t *testing.T) {
	model := newFakeTTSModel(t)
	client, err := tts.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}

	bytes, _, err := client.SynthesizeWithText("hi").Call().Speech(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != "audio" {
		t.Fatalf("Speech = %q", bytes)
	}
}

func TestClient_StreamSpeech_YieldsChunks(t *testing.T) {
	model := newFakeTTSModel(t)

	chunk := func(b string) *tts.Response {
		res, _ := tts.NewResult([]byte(b), &tts.ResultMetadata{})
		resp, _ := tts.NewResponse(res, &tts.ResponseMetadata{})
		return resp
	}
	model.streamYield = []*tts.Response{chunk("a"), chunk("b"), chunk("c")}

	client, _ := tts.NewClient(model)

	count := 0
	for chunk, err := range client.SynthesizeWithText("hi").Stream().Speech(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		_ = chunk
		count++
	}
	if count != 3 {
		t.Fatalf("got %d chunks, want 3", count)
	}
}

func TestClient_StreamSpeech_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeTTSModel(t)
	model.streamErr = want
	client, _ := tts.NewClient(model)

	gotErr := error(nil)
	for _, err := range client.SynthesizeWithText("hi").Stream().Response(context.Background()) {
		gotErr = err
		break
	}
	if !errors.Is(gotErr, want) {
		t.Fatalf("err = %v", gotErr)
	}
}

func TestNewClient_RejectsNilModel(t *testing.T) {
	if _, err := tts.NewClient(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestMergeOptions_RejectsNilBase(t *testing.T) {
	if _, err := tts.MergeOptions(nil); err == nil {
		t.Fatal("nil base must error")
	}
}
