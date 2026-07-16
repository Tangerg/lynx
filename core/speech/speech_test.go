package speech_test

import (
	"context"
	"errors"
	"iter"
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/speech"
)

func TestModelAndStreamerFunc(t *testing.T) {
	want := errors.New("boom")
	model := speech.ModelFunc(func(_ context.Context, request *speech.Request) (*speech.Response, error) {
		if request.Text != "hello" {
			t.Fatalf("text = %q", request.Text)
		}
		return nil, want
	})
	request, _ := speech.NewRequest("hello")
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}

	streamer := speech.StreamerFunc(func(context.Context, *speech.Request) iter.Seq2[*speech.Response, error] {
		return func(yield func(*speech.Response, error) bool) {
			yield(nil, want)
		}
	})
	for _, err := range streamer.Stream(t.Context(), request) {
		if !errors.Is(err, want) {
			t.Fatalf("Stream error = %v, want %v", err, want)
		}
		return
	}
	t.Fatal("Stream yielded nothing")
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := speech.NewOptions(""); err == nil || err.Error() != "speech.NewOptions: model id must not be empty" {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := speech.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	if _, err := (*speech.Options)(nil).Merged(); err == nil || err.Error() != "speech.Options.Merged: nil receiver" {
		t.Fatalf("Merged error = %v", err)
	}
	if err := (*speech.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &speech.Request{
		Text:    "text",
		Options: &speech.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &speech.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	for _, tc := range []struct {
		name  string
		speed float64
	}{
		{name: "negative", speed: -1},
		{name: "nan", speed: math.NaN()},
		{name: "positive infinity", speed: math.Inf(1)},
		{name: "negative infinity", speed: math.Inf(-1)},
	} {
		t.Run(tc.name+" speed", func(t *testing.T) {
			invalid.Options = &speech.Options{Speed: tc.speed}
			if err := invalid.Validate(); err == nil {
				t.Fatalf("Validate accepted speed %v", tc.speed)
			}
		})
	}
	options := new(speech.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
	if _, err := (&speech.Options{Model: "base", Speed: math.NaN()}).Merged(); err == nil {
		t.Fatal("Merged accepted invalid base options")
	}
}

func TestResponseValidation(t *testing.T) {
	if _, err := speech.NewResult(nil, &speech.ResultMetadata{}); err == nil || err.Error() != "speech.NewResult: audio must not be empty" {
		t.Fatalf("NewResult empty audio error = %v", err)
	}
	if _, err := speech.NewResult([]byte("audio"), nil); err == nil || err.Error() != "speech.NewResult: metadata must not be nil" {
		t.Fatalf("NewResult nil metadata error = %v", err)
	}
	result, _ := speech.NewResult([]byte("audio"), &speech.ResultMetadata{})
	if _, err := speech.NewResponse(result, &speech.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndCopies(t *testing.T) {
	if clone := (*speech.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	base := &speech.Options{Model: "base", Voice: "base-voice", Extra: mustMetadata(t, map[string]any{"base": true})}
	merged, err := base.Merged(nil, &speech.Options{
		Model: "override", Voice: "alloy", OutputFormat: "mp3", Speed: 1.25,
		Extra: mustMetadata(t, map[string]any{"override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Voice != "alloy" || merged.OutputFormat != "mp3" ||
		merged.Speed != 1.25 || len(merged.Extra) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	clone := merged.Clone()
	if err := clone.Extra.Set("base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, merged.Extra, "base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	result, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustDecode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := metadata.Decode[T](values, key)
	if err != nil || !ok {
		t.Fatalf("metadata.Decode(%q) = %#v, %t, %v", key, value, ok, err)
	}
	return value
}

func TestResponseAndRequestErrorBoundaries(t *testing.T) {
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	audio := []byte("audio")
	result, _ := speech.NewResult(audio, &speech.ResultMetadata{})
	audio[0] = 'X'
	if string(result.Audio) != "audio" {
		t.Fatal("NewResult aliases caller audio")
	}
	if _, err := speech.NewResponse(nil, &speech.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := speech.NewResponse(result, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	if err := (&speech.Response{Result: result, Metadata: &speech.ResponseMetadata{Created: -1}}).Validate(); err == nil {
		t.Fatal("Validate accepted a negative creation time")
	}
}
