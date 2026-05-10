package transcription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/pkg/mime"
)

type fakeTranscriptionModel struct {
	defaults *transcription.Options
	lastReq  *transcription.Request
	respond  func(req *transcription.Request) (*transcription.Response, error)
}

func newFakeTranscriptionModel(t *testing.T) *fakeTranscriptionModel {
	t.Helper()
	o, err := transcription.NewOptions("whisper-1")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeTranscriptionModel{defaults: o}
}

func (m *fakeTranscriptionModel) DefaultOptions() *transcription.Options { return m.defaults }
func (m *fakeTranscriptionModel) Info() transcription.ModelInfo {
	return transcription.ModelInfo{Provider: "fake"}
}

func (m *fakeTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	res, _ := transcription.NewResult("hi", &transcription.ResultMetadata{})
	return transcription.NewResponse([]*transcription.Result{res}, &transcription.ResponseMetadata{})
}

func mustAudio(t *testing.T) *media.Media {
	t.Helper()
	mt, err := mime.Parse("audio/mpeg")
	if err != nil {
		t.Fatal(err)
	}
	m, err := media.NewMedia(mt, []byte("audio-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNewOptions_RequiresModel(t *testing.T) {
	if _, err := transcription.NewOptions(""); err == nil {
		t.Fatal("empty model must error")
	}
}

func TestNewRequest_RequiresAudio(t *testing.T) {
	if _, err := transcription.NewRequest(nil); err == nil {
		t.Fatal("nil audio must error")
	}
}

func TestNewResult_AllowsEmptyText(t *testing.T) {
	if _, err := transcription.NewResult("", &transcription.ResultMetadata{}); err != nil {
		t.Fatalf("empty text should be allowed: %v", err)
	}
	if _, err := transcription.NewResult("text", nil); err == nil {
		t.Fatal("nil metadata must error")
	}
}

func TestNewClient_RejectsNilModel(t *testing.T) {
	if _, err := transcription.NewClient(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestClient_TranscribeWithAudio_ReturnsText(t *testing.T) {
	model := newFakeTranscriptionModel(t)
	client, err := transcription.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}

	text, _, err := client.TranscribeWithAudio(mustAudio(t)).Call().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "hi" {
		t.Fatalf("text = %q", text)
	}
}

func TestClient_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeTranscriptionModel(t)
	model.respond = func(*transcription.Request) (*transcription.Response, error) { return nil, want }

	client, _ := transcription.NewClient(model)
	if _, err := client.TranscribeWithAudio(mustAudio(t)).Call().Response(context.Background()); !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestMergeOptions_RejectsNilBase(t *testing.T) {
	if _, err := transcription.MergeOptions(nil); err == nil {
		t.Fatal("nil base must error")
	}
}
