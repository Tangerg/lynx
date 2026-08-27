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
	if _, err := speech.NewOptions(""); !errors.Is(err, speech.ErrInvalidOptions) {
		t.Fatalf("NewOptions error = %v", err)
	}
	if _, err := speech.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := speech.NewRequest(""); err == nil {
		t.Fatal("NewRequest accepted empty text")
	}
	if resolved, err := (speech.Options{}).Resolve(speech.Options{}); err != nil || resolved.Model != "" || resolved.Speed != 0 || len(resolved.Extensions) != 0 {
		t.Fatalf("zero Options.Resolve(empty) = %#v, %v", resolved, err)
	}
	if err := (*speech.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	invalid := &speech.Request{
		Text:    "text",
		Options: speech.Options{Extensions: metadata.Map{"provider/broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = speech.Options{Model: " model "}
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
			invalid.Options = speech.Options{Speed: tc.speed}
			if err := invalid.Validate(); err == nil {
				t.Fatalf("Validate accepted speed %v", tc.speed)
			}
		})
	}
	options := new(speech.Options)
	if err := options.SetExtension("provider/value", func() {}); err == nil || options.Extensions != nil {
		t.Fatalf("failed SetExtension mutated options: %#v, %v", options.Extensions, err)
	}
	if _, err := (speech.Options{Model: "base", Speed: math.NaN()}).Resolve(speech.Options{}); err == nil {
		t.Fatal("Resolve accepted invalid base options")
	}
}

func TestResponseValidation(t *testing.T) {
	if _, err := speech.NewOutput(nil, &speech.OutputMetadata{}); !errors.Is(err, speech.ErrInvalidResponse) {
		t.Fatalf("NewOutput empty audio error = %v", err)
	}
	if _, err := speech.NewOutput([]byte("audio"), nil); !errors.Is(err, speech.ErrInvalidResponse) {
		t.Fatalf("NewOutput nil metadata error = %v", err)
	}
	output, _ := speech.NewOutput([]byte("audio"), &speech.OutputMetadata{})
	if _, err := speech.NewResponse(output, &speech.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsResolveAndCopies(t *testing.T) {
	base := speech.Options{Model: "base", Voice: "base-voice", Extensions: mustMetadata(t, map[string]any{"provider/base": true})}
	resolved, err := base.Resolve(speech.Options{
		Model: "override", Voice: "alloy", OutputFormat: "mp3", Speed: 1.25,
		Extensions: mustMetadata(t, map[string]any{"provider/override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model != "override" || resolved.Voice != "alloy" || resolved.OutputFormat != "mp3" ||
		resolved.Speed != 1.25 || len(resolved.Extensions) != 2 {
		t.Fatalf("Resolve = %#v", resolved)
	}
	clone := resolved.Clone()
	if err := clone.Extensions.Set("provider/base", false); err != nil {
		t.Fatal(err)
	}
	if !mustDecode[bool](t, resolved.Extensions, "provider/base") {
		t.Fatal("Options.Clone aliases source state")
	}
}

func mustMetadata(t *testing.T, values map[string]any) metadata.Map {
	t.Helper()
	output, err := metadata.FromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func mustDecode[T any](t *testing.T, values metadata.Map, key string) T {
	t.Helper()
	value, ok, err := values.Decode[T](key)
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
	output, _ := speech.NewOutput(audio, &speech.OutputMetadata{})
	audio[0] = 'X'
	if string(output.Audio) != "audio" {
		t.Fatal("NewOutput aliases caller audio")
	}
	if _, err := speech.NewResponse(nil, &speech.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil output")
	}
	if _, err := speech.NewResponse(output, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
	if err := (&speech.Response{Output: output, Metadata: &speech.ResponseMetadata{Created: -1}}).Validate(); err == nil {
		t.Fatal("Validate accepted a negative creation time")
	}
}
