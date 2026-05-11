package openai

import (
	"context"
	"net/http"
	"slices"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey         model.ApiKey
	RequestOptions []option.RequestOption
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.ApiKey == nil {
		return ErrMissingApiKey
	}
	return nil
}

type Api struct {
	client *openai.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Clone caller's slice and append the API-key option last so it
	// can't be overridden by an earlier WithAPIKey on the original
	// slice. Cloning prevents append from mutating the caller's
	// backing array when capacity allows.
	options := append(slices.Clone(cfg.RequestOptions), option.WithAPIKey(cfg.ApiKey.Get()))

	return &Api{client: new(openai.NewClient(options...))}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Chat.Completions.New(ctx, *req, opts...)
}

func (a *Api) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Chat.Completions.NewStreaming(ctx, *req, opts...), nil
}

func (a *Api) Embedding(ctx context.Context, req *openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Embeddings.New(ctx, *req, opts...)
}

func (a *Api) Image(ctx context.Context, req *openai.ImageGenerateParams, opts ...option.RequestOption) (*openai.ImagesResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Images.Generate(ctx, *req, opts...)
}

func (a *Api) Moderation(ctx context.Context, req *openai.ModerationNewParams, opts ...option.RequestOption) (*openai.ModerationNewResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Moderations.New(ctx, *req, opts...)
}

func (a *Api) AudioTTS(ctx context.Context, req *openai.AudioSpeechNewParams, opts ...option.RequestOption) (*http.Response, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Audio.Speech.New(ctx, *req, opts...)
}

func (a *Api) AudioTranscription(ctx context.Context, req *openai.AudioTranscriptionNewParams, opts ...option.RequestOption) (*openai.AudioTranscriptionNewResponseUnion, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	return a.client.Audio.Transcriptions.New(ctx, *req, opts...)
}
