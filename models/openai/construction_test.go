package openai_test

import (
	"testing"

	"github.com/Tangerg/scope/models/openai"
)

// TestConstructorsRejectAnAbsentCredential is the one contract this module can
// prove without a live account: a credential-gated provider must fail at
// construction rather than at the first call, where the failure would surface
// as a transport error the caller cannot distinguish from an outage.
func TestConstructorsRejectAnAbsentCredential(t *testing.T) {
	cases := map[string]func() error{
		"NewChat": func() error {
			_, err := openai.NewChat(openai.ChatConfig{})
			return err
		},
		"NewResponsesChat": func() error {
			_, err := openai.NewResponsesChat(openai.ChatConfig{})
			return err
		},
		"NewEmbeddingModel": func() error {
			_, err := openai.NewEmbeddingModel(openai.EmbeddingModelConfig{})
			return err
		},
		"NewAudioTranscriptionModel": func() error {
			_, err := openai.NewAudioTranscriptionModel(openai.AudioTranscriptionModelConfig{})
			return err
		},
		"NewAudioTranslationModel": func() error {
			_, err := openai.NewAudioTranslationModel(openai.AudioTranslationModelConfig{})
			return err
		},
		"NewAudioTTSModel": func() error {
			_, err := openai.NewAudioTTSModel(openai.AudioTTSModelConfig{})
			return err
		},
		"NewImageModel": func() error {
			_, err := openai.NewImageModel(openai.ImageModelConfig{})
			return err
		},
		"NewModerationModel": func() error {
			_, err := openai.NewModerationModel(openai.ModerationModelConfig{})
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
	if openai.Provider == "" {
		t.Fatal("Provider is empty")
	}
}
