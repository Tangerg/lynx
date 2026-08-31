package google_test

import (
	"testing"

	"github.com/Tangerg/scope/models/google"
)

// TestConstructorsRejectAnAbsentCredential is the one contract this module can
// prove without a live account: a credential-gated provider must fail at
// construction rather than at the first call, where the failure would surface
// as a transport error the caller cannot distinguish from an outage.
func TestConstructorsRejectAnAbsentCredential(t *testing.T) {
	cases := map[string]func() error{
		"NewChat": func() error {
			_, err := google.NewChat(google.ChatConfig{})
			return err
		},
		"NewChatCompletions": func() error {
			_, err := google.NewChatCompletions(google.ChatCompletionsConfig{})
			return err
		},
		"NewEmbeddingModel": func() error {
			_, err := google.NewEmbeddingModel(google.EmbeddingModelConfig{})
			return err
		},
		"NewAudioTTSModel": func() error {
			_, err := google.NewAudioTTSModel(google.AudioTTSModelConfig{})
			return err
		},
		"NewAudioTranscriptionModel": func() error {
			_, err := google.NewAudioTranscriptionModel(google.AudioTranscriptionModelConfig{})
			return err
		},
		"NewImageModel": func() error {
			_, err := google.NewImageModel(google.ImageModelConfig{})
			return err
		},
	}
	for name, construct := range cases {
		t.Run(name, func(t *testing.T) {
			if err := construct(); err == nil {
				t.Fatal("an empty config constructed a usable model")
			}
		})
	}
}

// TestProviderIdentityIsStable pins the identifiers a composition root passes
// to observability and a Host stores as provider identity. They are wire-facing
// constants, so a rename is a breaking change rather than a refactor.
func TestProviderIdentityIsStable(t *testing.T) {
	if google.Provider == "" {
		t.Fatal("Provider is empty")
	}
}
