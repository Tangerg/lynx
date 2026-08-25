package arch

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/moderation"
	"github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
)

func TestModalityModelsExposeConsistentOwnedBehavior(t *testing.T) {
	t.Parallel()

	modalities := []struct {
		name             string
		options          any
		request          any
		resultMetadata   any
		result           any
		responseMetadata any
		response         any
	}{
		{"chat", chat.Options{}, chat.Request{}, chat.ResultMetadata{}, chat.Result{}, chat.ResponseMetadata{}, chat.Response{}},
		{"embedding", embedding.Options{}, embedding.Request{}, embedding.ResultMetadata{}, embedding.Result{}, embedding.ResponseMetadata{}, embedding.Response{}},
		{"image", image.Options{}, image.Request{}, image.ResultMetadata{}, image.Result{}, image.ResponseMetadata{}, image.Response{}},
		{"moderation", moderation.Options{}, moderation.Request{}, moderation.ResultMetadata{}, moderation.Result{}, moderation.ResponseMetadata{}, moderation.Response{}},
		{"speech", speech.Options{}, speech.Request{}, speech.ResultMetadata{}, speech.Result{}, speech.ResponseMetadata{}, speech.Response{}},
		{"transcription", transcription.Options{}, transcription.Request{}, transcription.ResultMetadata{}, transcription.Result{}, transcription.ResponseMetadata{}, transcription.Response{}},
	}

	for _, modality := range modalities {
		t.Run(modality.name, func(t *testing.T) {
			assertMethods(t, reflect.TypeOf(modality.options), "Clone", "MarshalJSON", "Merged", "Validate")
			assertPointerMethods(t, modality.options, "SetExtension", "UnmarshalJSON")

			assertMethods(t, reflect.TypeOf(modality.request), "MarshalJSON")
			assertPointerMethods(t, modality.request, "UnmarshalJSON", "Validate")

			assertMethods(t, reflect.TypeOf(modality.resultMetadata), "MarshalJSON")
			assertPointerMethods(t, modality.resultMetadata, "Set", "UnmarshalJSON")

			assertMethods(t, reflect.TypeOf(modality.result), "MarshalJSON")
			assertPointerMethods(t, modality.result, "UnmarshalJSON", "Validate")

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
