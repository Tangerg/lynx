package openai

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"

	"github.com/Tangerg/lynx/core/model"
)

type APIConfig struct {
	APIKey         model.APIKey
	RequestOptions []option.RequestOption
}

func (c *APIConfig) validate() error {
	if c == nil {
		return errors.New("openai: config must not be nil")
	}
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	return nil
}

type API struct {
	client *openai.Client
}

func NewAPI(cfg *APIConfig) (*API, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Clone caller's slice and append the API-key option last so it
	// can't be overridden by an earlier WithAPIKey on the original
	// slice. Cloning prevents append from mutating the caller's
	// backing array when capacity allows.
	options := append(slices.Clone(cfg.RequestOptions), option.WithAPIKey(cfg.APIKey.Get()))

	return &API{client: new(openai.NewClient(options...))}, nil
}

func (a *API) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Chat.Completions.New(ctx, *req, opts...)
}

func (a *API) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Chat.Completions.NewStreaming(ctx, *req, opts...), nil
}

func (a *API) ResponseNew(ctx context.Context, req *responses.ResponseNewParams, opts ...option.RequestOption) (*responses.Response, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Responses.New(ctx, *req, opts...)
}

func (a *API) ResponseNewStream(ctx context.Context, req *responses.ResponseNewParams, opts ...option.RequestOption) (*ssestream.Stream[responses.ResponseStreamEventUnion], error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Responses.NewStreaming(ctx, *req, opts...), nil
}

func (a *API) Embedding(ctx context.Context, req *openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Embeddings.New(ctx, *req, opts...)
}

func (a *API) Image(ctx context.Context, req *openai.ImageGenerateParams, opts ...option.RequestOption) (*openai.ImagesResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Images.Generate(ctx, *req, opts...)
}

func (a *API) Moderation(ctx context.Context, req *openai.ModerationNewParams, opts ...option.RequestOption) (*openai.ModerationNewResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Moderations.New(ctx, *req, opts...)
}

func (a *API) AudioTTS(ctx context.Context, req *openai.AudioSpeechNewParams, opts ...option.RequestOption) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Audio.Speech.New(ctx, *req, opts...)
}

func (a *API) AudioTranscription(ctx context.Context, req *openai.AudioTranscriptionNewParams, opts ...option.RequestOption) (*openai.AudioTranscriptionNewResponseUnion, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Audio.Transcriptions.New(ctx, *req, opts...)
}

func (a *API) AudioTranslation(ctx context.Context, req *openai.AudioTranslationNewParams, opts ...option.RequestOption) (*openai.Translation, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Audio.Translations.New(ctx, *req, opts...)
}
