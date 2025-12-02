package openai

import (
	"context"
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"

	"github.com/Tangerg/lynx/ai/model"
)

type ApiConfig struct {
	ApiKey         model.ApiKey
	RequestOptions []option.RequestOption
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("apiKey is required")
	}
	return nil
}

type Api struct {
	apiKey model.ApiKey
	client *openai.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// ensure apikey at last
	options := append(cfg.RequestOptions, option.WithAPIKey(cfg.ApiKey.Get()))
	client := openai.NewClient(options...)

	return &Api{
		apiKey: cfg.ApiKey,
		client: &client,
	}, nil
}

func (a *Api) ChatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Chat.Completions.New(ctx, *req, opts...)
}

func (a *Api) ChatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Chat.Completions.NewStreaming(ctx, *req, opts...), nil
}

func (a *Api) Embedding(ctx context.Context, req *openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Embeddings.New(ctx, *req, opts...)
}

func (a *Api) Image(ctx context.Context, req *openai.ImageGenerateParams, opts ...option.RequestOption) (*openai.ImagesResponse, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Images.Generate(ctx, *req, opts...)
}

func (a *Api) Moderation(ctx context.Context, req *openai.ModerationNewParams, opts ...option.RequestOption) (*openai.ModerationNewResponse, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Moderations.New(ctx, *req, opts...)
}

func (a *Api) AudioTextToSpeech(ctx context.Context, req *openai.AudioSpeechNewParams, opts ...option.RequestOption) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Audio.Speech.New(ctx, *req, opts...)
}

func (a *Api) AudioTranscription(ctx context.Context, req *openai.AudioTranscriptionNewParams, opts ...option.RequestOption) (*openai.AudioTranscriptionNewResponseUnion, error) {
	if req == nil {
		return nil, errors.New("request parameters cannot be nil")
	}
	return a.client.Audio.Transcriptions.New(ctx, *req, opts...)
}
