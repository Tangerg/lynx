package arch

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
)

// TestModalityWireValuesValidateAtJSONBoundary keeps invariant-bearing
// modality values from silently falling back to encoding/json's field-only
// behavior. Each value must reject invalid state on both encode and decode.
func TestModalityWireValuesValidateAtJSONBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		marshaler   any
		unmarshaler any
	}{
		{"embedding.Options", embedding.Options{}, new(embedding.Options)},
		{"embedding.Request", embedding.Request{}, new(embedding.Request)},
		{"embedding.ResultMetadata", embedding.ResultMetadata{}, new(embedding.ResultMetadata)},
		{"embedding.Result", embedding.Result{}, new(embedding.Result)},
		{"embedding.Usage", embedding.Usage{}, new(embedding.Usage)},
		{"embedding.ResponseMetadata", embedding.ResponseMetadata{}, new(embedding.ResponseMetadata)},
		{"embedding.Response", embedding.Response{}, new(embedding.Response)},

		{"image.Options", image.Options{}, new(image.Options)},
		{"image.Request", image.Request{}, new(image.Request)},
		{"image.ResultMetadata", image.ResultMetadata{}, new(image.ResultMetadata)},
		{"image.Result", image.Result{}, new(image.Result)},
		{"image.ResponseMetadata", image.ResponseMetadata{}, new(image.ResponseMetadata)},
		{"image.Response", image.Response{}, new(image.Response)},

		{"speech.Options", speech.Options{}, new(speech.Options)},
		{"speech.Request", speech.Request{}, new(speech.Request)},
		{"speech.ResultMetadata", speech.ResultMetadata{}, new(speech.ResultMetadata)},
		{"speech.Result", speech.Result{}, new(speech.Result)},
		{"speech.ResponseMetadata", speech.ResponseMetadata{}, new(speech.ResponseMetadata)},
		{"speech.Response", speech.Response{}, new(speech.Response)},

		{"transcription.Options", transcription.Options{}, new(transcription.Options)},
		{"transcription.Request", transcription.Request{}, new(transcription.Request)},
		{"transcription.ResultMetadata", transcription.ResultMetadata{}, new(transcription.ResultMetadata)},
		{"transcription.Result", transcription.Result{}, new(transcription.Result)},
		{"transcription.ResponseMetadata", transcription.ResponseMetadata{}, new(transcription.ResponseMetadata)},
		{"transcription.Response", transcription.Response{}, new(transcription.Response)},

		{"moderation.Options", moderation.Options{}, new(moderation.Options)},
		{"moderation.Request", moderation.Request{}, new(moderation.Request)},
		{"moderation.Verdict", moderation.Verdict{}, new(moderation.Verdict)},
		{"moderation.Categories", moderation.Categories{}, new(moderation.Categories)},
		{"moderation.ResultMetadata", moderation.ResultMetadata{}, new(moderation.ResultMetadata)},
		{"moderation.Result", moderation.Result{}, new(moderation.Result)},
		{"moderation.ResponseMetadata", moderation.ResponseMetadata{}, new(moderation.ResponseMetadata)},
		{"moderation.Response", moderation.Response{}, new(moderation.Response)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := test.marshaler.(json.Marshaler); !ok {
				t.Errorf("%s does not implement json.Marshaler", test.name)
			}
			if _, ok := test.unmarshaler.(json.Unmarshaler); !ok {
				t.Errorf("*%s does not implement json.Unmarshaler", test.name)
			}
		})
	}
}
