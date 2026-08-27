package arch

import (
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/core/moderation"
	"github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/core/transcription"
)

func TestModalityModelsExposeConsistentOwnedBehavior(t *testing.T) {
	t.Parallel()

	modalities := []struct {
		name             string
		options          any
		request          any
		resultMetadata   any
		output           any
		responseMetadata any
		response         any
	}{
		{"chat", chat.Options{}, chat.Request{}, chat.OutputMetadata{}, chat.Output{}, chat.ResponseMetadata{}, chat.Response{}},
		{"embedding", embedding.Options{}, embedding.Request{}, embedding.OutputMetadata{}, embedding.Output{}, embedding.ResponseMetadata{}, embedding.Response{}},
		{"image", image.Options{}, image.Request{}, image.OutputMetadata{}, image.Output{}, image.ResponseMetadata{}, image.Response{}},
		{"moderation", moderation.Options{}, moderation.Request{}, moderation.OutputMetadata{}, moderation.Output{}, moderation.ResponseMetadata{}, moderation.Response{}},
		{"speech", speech.Options{}, speech.Request{}, speech.OutputMetadata{}, speech.Output{}, speech.ResponseMetadata{}, speech.Response{}},
		{"transcription", transcription.Options{}, transcription.Request{}, transcription.OutputMetadata{}, transcription.Output{}, transcription.ResponseMetadata{}, transcription.Response{}},
	}

	for _, modality := range modalities {
		t.Run(modality.name, func(t *testing.T) {
			assertMethods(t, reflect.TypeOf(modality.options), "Clone", "MarshalJSON", "Resolve", "Validate")
			assertPointerMethods(t, modality.options, "SetExtension", "UnmarshalJSON")

			assertMethods(t, reflect.TypeOf(modality.request), "MarshalJSON")
			assertPointerMethods(t, modality.request, "UnmarshalJSON", "Validate")

			assertMethods(t, reflect.TypeOf(modality.resultMetadata), "MarshalJSON")
			assertPointerMethods(t, modality.resultMetadata, "Set", "UnmarshalJSON")

			assertMethods(t, reflect.TypeOf(modality.output), "MarshalJSON")
			assertPointerMethods(t, modality.output, "UnmarshalJSON", "Validate")

			assertMethods(t, reflect.TypeOf(modality.responseMetadata), "MarshalJSON")
			assertPointerMethods(t, modality.responseMetadata, "Set", "UnmarshalJSON")

			assertMethods(t, reflect.TypeOf(modality.response), "MarshalJSON")
			assertPointerMethods(t, modality.response, "UnmarshalJSON", "Validate")
		})
	}
}

func assertPointerMethods(t *testing.T, value any, methods ...string) {
	t.Helper()
	assertMethods(t, reflect.PointerTo(reflect.TypeOf(value)), methods...)
}

func assertMethods(t *testing.T, valueType reflect.Type, methods ...string) {
	t.Helper()
	for _, method := range methods {
		if _, found := valueType.MethodByName(method); !found {
			t.Errorf("%s does not own %s", valueType, method)
		}
	}
}
