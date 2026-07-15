package transcription_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
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
	if _, err := transcription.NewRequest(nil); err == nil {
		t.Fatal("NewRequest accepted nil audio")
	}
	if _, err := transcription.MergeOptions(nil); err == nil {
		t.Fatal("MergeOptions accepted nil base")
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

func TestOptionsMergeAndProtocolAccessors(t *testing.T) {
	if _, ok := (*transcription.Options)(nil).Get("missing"); ok {
		t.Fatal("nil Options reported a value")
	}
	if clone := (*transcription.Options)(nil).Clone(); clone != nil {
		t.Fatalf("nil Clone = %#v", clone)
	}
	temperature := 0.2
	base := &transcription.Options{
		Model: "base", TimestampGranularity: []string{"segment"}, Extra: map[string]any{"base": true},
	}
	merged, err := transcription.MergeOptions(base, nil, &transcription.Options{
		Model: "override", Language: "en", Prompt: "Lynx", Temperature: &temperature,
		ResponseFormat: "verbose_json", TimestampGranularity: []string{"word"},
		Extra: map[string]any{"override": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Model != "override" || merged.Language != "en" || merged.Prompt != "Lynx" ||
		merged.Temperature == nil || merged.ResponseFormat != "verbose_json" ||
		len(merged.TimestampGranularity) != 1 || merged.TimestampGranularity[0] != "word" || len(merged.Extra) != 2 {
		t.Fatalf("MergeOptions = %#v", merged)
	}
	clone := merged.Clone()
	*clone.Temperature = 0.9
	clone.TimestampGranularity[0] = "segment"
	clone.Extra["base"] = false
	if *merged.Temperature != 0.2 || merged.TimestampGranularity[0] != "word" || merged.Extra["base"] != true {
		t.Fatal("Options.Clone aliases source state")
	}
	merged.Set("region", "local")
	if value, ok := merged.Get("region"); !ok || value != "local" {
		t.Fatalf("Options.Get = %#v, %t", value, ok)
	}

	resultMetadata := &transcription.ResultMetadata{}
	resultMetadata.Set("duration", 1.5)
	if value, ok := resultMetadata.Get("duration"); !ok || value != 1.5 {
		t.Fatalf("ResultMetadata.Get = %#v, %t", value, ok)
	}
	responseMetadata := &transcription.ResponseMetadata{}
	responseMetadata.Set("region", "local")
	if value, ok := responseMetadata.Get("region"); !ok || value != "local" {
		t.Fatalf("ResponseMetadata.Get = %#v, %t", value, ok)
	}
	if _, ok := (*transcription.ResultMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResultMetadata reported a value")
	}
	if _, ok := (*transcription.ResponseMetadata)(nil).Get("missing"); ok {
		t.Fatal("nil ResponseMetadata reported a value")
	}
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
