package transcription_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/transcription"
)

func TestModelFunc(t *testing.T) {
	want := errors.New("boom")
	model := transcription.ModelFunc(func(_ context.Context, request *transcription.Request) (*transcription.Response, error) {
		if request.Audio == nil {
			t.Fatal("audio is nil")
		}
		return nil, want
	})
	audio, err := media.NewBytes("audio/mpeg", []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	request, _ := transcription.NewRequest(audio)
	if _, err := model.Call(t.Context(), request); !errors.Is(err, want) {
		t.Fatalf("Call error = %v, want %v", err, want)
	}
}

func TestOptionsAndRequestValidation(t *testing.T) {
	if _, err := transcription.NewOptions(""); err == nil {
		t.Fatal("NewOptions accepted empty model")
	}
	if _, err := transcription.NewOptions(" model "); err == nil {
		t.Fatal("NewOptions accepted model with surrounding whitespace")
	}
	if _, err := transcription.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted nil audio")
	}
	if _, err := (*transcription.Options)(nil).Merged(); err == nil {
		t.Fatal("Merged accepted nil receiver")
	}
	if err := (*transcription.Request)(nil).Validate(); err == nil {
		t.Fatal("Validate accepted nil request")
	}
	audio, err := media.NewBytes("audio/mpeg", []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	invalid := &transcription.Request{
		Audio:   audio,
		Options: &transcription.Options{Extra: metadata.Map{"broken": []byte("{")}},
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted invalid options metadata")
	}
	invalid.Options = &transcription.Options{Model: " model "}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted model with surrounding whitespace")
	}
	for _, tc := range []struct {
		name    string
		value   float64
		wantErr bool
	}{
		{name: "negative", value: -0.1, wantErr: true},
		{name: "zero", value: 0},
		{name: "one", value: 1},
		{name: "above one", value: 1.1, wantErr: true},
		{name: "nan", value: math.NaN(), wantErr: true},
		{name: "infinity", value: math.Inf(1), wantErr: true},
	} {
		t.Run(tc.name+" temperature", func(t *testing.T) {
			invalid.Options = &transcription.Options{Temperature: new(tc.value)}
			err := invalid.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate temperature %v error = %v, wantErr %t", tc.value, err, tc.wantErr)
			}
		})
	}
	for _, granularity := range []string{"", " word "} {
		invalid.Options = &transcription.Options{TimestampGranularity: []string{granularity}}
		if err := invalid.Validate(); err == nil {
			t.Errorf("Validate accepted timestamp granularity %q", granularity)
		}
	}
	options := new(transcription.Options)
	if err := options.Set("provider/value", func() {}); err == nil || options.Extra != nil {
		t.Fatalf("failed Set mutated options: %#v, %v", options.Extra, err)
	}
}

func TestResponseValidation(t *testing.T) {
	result, err := transcription.NewResult("", &transcription.ResultMetadata{})
	if err != nil {
		t.Fatalf("NewResult rejected empty transcript: %v", err)
	}
	if _, err := transcription.NewResult("text", nil); err == nil {
		t.Fatal("NewResult accepted nil metadata")
	}
	if _, err := transcription.NewResponse(result, &transcription.ResponseMetadata{}); err != nil {
		t.Fatal(err)
	}
}

func TestOptionsMergeAndCopies(t *testing.T) {
	if clone := (*transcription.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	temperature := 0.2
	base := &transcription.Options{
		Model: "base", TimestampGranularity: []string{"segment"},
		Extra: mustMetadata(t, map[string]any{"base": true}),
	}
	merged, err := base.Merged(nil, &transcription.Options{
		Model: "override", Language: "en", Prompt: "Lynx", Temperature: &temperature,
		ResponseFormat: "verbose_json", TimestampGranularity: []string{"word"},
		Extra: mustMetadata(t, map[string]any{"override": true}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Language != "en" || merged.Prompt != "Lynx" ||
		merged.Temperature == nil || merged.ResponseFormat != "verbose_json" ||
		len(merged.TimestampGranularity) != 1 || merged.TimestampGranularity[0] != "word" || len(merged.Extra) != 2 {
		t.Fatalf("Merged = %#v", merged)
	}
	temperature = 0.4
	if *merged.Temperature != 0.2 {
		t.Fatal("Merged aliases override pointer state")
	}
	clone := merged.Clone()
	*clone.Temperature = 0.9
	clone.TimestampGranularity[0] = "segment"
	if err := clone.Extra.Set("base", false); err != nil {
		t.Fatal(err)
	}
	if *merged.Temperature != 0.2 || merged.TimestampGranularity[0] != "word" ||
		!mustDecode[bool](t, merged.Extra, "base") {
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

func TestResponseErrorBoundaries(t *testing.T) {
	result, _ := transcription.NewResult("lynx", &transcription.ResultMetadata{})
	if _, err := transcription.NewResponse(nil, &transcription.ResponseMetadata{}); err == nil {
		t.Fatal("NewResponse accepted nil result")
	}
	if _, err := transcription.NewResponse(result, nil); err == nil {
		t.Fatal("NewResponse accepted nil metadata")
	}
}
