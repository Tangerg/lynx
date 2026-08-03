package openai

import (
	"context"
	"errors"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

type apiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Headers    http.Header
}

func (c apiConfig) validate() error {
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	return nil
}

type api struct {
	client *openai.Client
}

func newAPI(cfg apiConfig) (*api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	options := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		options = append(options, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(cfg.HTTPClient))
	}
	for name, values := range cfg.Headers {
		for _, value := range values {
			options = append(options, option.WithHeader(name, value))
		}
	}
	client := openai.NewClient(options...)

	return &api{client: &client}, nil
}

func (a *api) chatCompletion(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Chat.Completions.New(ctx, *req, opts...))
}

func (a *api) chatCompletionStream(ctx context.Context, req *openai.ChatCompletionNewParams, opts ...option.RequestOption) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Chat.Completions.NewStreaming(ctx, *req, opts...), nil
}

func (a *api) responseNew(ctx context.Context, req *responses.ResponseNewParams, opts ...option.RequestOption) (*responses.Response, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Responses.New(ctx, *req, opts...))
}

func (a *api) responseNewStream(ctx context.Context, req *responses.ResponseNewParams, opts ...option.RequestOption) (*ssestream.Stream[responses.ResponseStreamEventUnion], error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return a.client.Responses.NewStreaming(ctx, *req, opts...), nil
}

func (a *api) embedding(ctx context.Context, req *openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Embeddings.New(ctx, *req, opts...))
}

func (a *api) image(ctx context.Context, req *openai.ImageGenerateParams, opts ...option.RequestOption) (*openai.ImagesResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Images.Generate(ctx, *req, opts...))
}

func (a *api) moderation(ctx context.Context, req *openai.ModerationNewParams, opts ...option.RequestOption) (*openai.ModerationNewResponse, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Moderations.New(ctx, *req, opts...))
}

func (a *api) audioTTS(ctx context.Context, req *openai.AudioSpeechNewParams, opts ...option.RequestOption) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Audio.Speech.New(ctx, *req, opts...))
}

func (a *api) audioTranscription(ctx context.Context, req *openai.AudioTranscriptionNewParams, opts ...option.RequestOption) (*openai.AudioTranscriptionNewResponseUnion, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Audio.Transcriptions.New(ctx, *req, opts...))
}

func (a *api) audioTranslation(ctx context.Context, req *openai.AudioTranslationNewParams, opts ...option.RequestOption) (*openai.Translation, error) {
	if req == nil {
		return nil, errors.New("openai: request must not be nil")
	}
	return wrapResult(a.client.Audio.Translations.New(ctx, *req, opts...))
}
