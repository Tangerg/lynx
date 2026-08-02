package openai

import (
	"context"
	"errors"
	"iter"

	"github.com/openai/openai-go/v3/option"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/moderation"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
	openaiprotocol "github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	Provider = "OpenAI"

	RequestExtensionKey     = "openai/request"
	ResponseExtensionKey    = "openai/response"
	StreamChunkExtensionKey = "openai/stream_chunk"

	ResponsesRequestExtensionKey  = "openai/responses_request"
	ResponsesResponseExtensionKey = "openai/responses_response"

	SpeechRequestExtensionKey        = "openai/speech_request"
	TranscriptionRequestExtensionKey = "openai/transcription_request"
	TranslationRequestExtensionKey   = "openai/translation_request"
	EmbeddingRequestExtensionKey     = "openai/embedding_request"
	ImageRequestExtensionKey         = "openai/image_request"
	ModerationRequestExtensionKey    = "openai/moderation_request"
)

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	RequestOptions []option.RequestOption
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() openaiprotocol.ChatConfig {
	return openaiprotocol.ChatConfig{
		APIKey:         c.APIKey,
		DefaultOptions: c.DefaultOptions,
		RequestOptions: c.RequestOptions,
	}
}

type Chat struct{ protocol *openaiprotocol.Chat }

func NewChat(config ChatConfig) (*Chat, error) {
	model, err := openaiprotocol.NewChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: model}, nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("openai: nil Chat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("openai: nil Chat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type ResponsesChat struct{ protocol *openaiprotocol.ResponsesChat }

func NewResponsesChat(config ChatConfig) (*ResponsesChat, error) {
	model, err := openaiprotocol.NewResponsesChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return &ResponsesChat{protocol: model}, nil
}

func (c *ResponsesChat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("openai: nil ResponsesChat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *ResponsesChat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("openai: nil ResponsesChat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	RequestOptions []option.RequestOption
}

func (c EmbeddingModelConfig) Validate() error { return c.protocol().Validate() }

func (c EmbeddingModelConfig) protocol() openaiprotocol.EmbeddingModelConfig {
	return openaiprotocol.EmbeddingModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type EmbeddingModel struct {
	protocol *openaiprotocol.EmbeddingModel
}

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	model, err := openaiprotocol.NewEmbeddingModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{protocol: model}, nil
}

func (m *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil EmbeddingModel")
	}
	return m.protocol.Call(ctx, req)
}

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranscriptionModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranscriptionModelConfig) protocol() openaiprotocol.AudioTranscriptionModelConfig {
	return openaiprotocol.AudioTranscriptionModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type AudioTranscriptionModel struct {
	protocol *openaiprotocol.AudioTranscriptionModel
}

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	model, err := openaiprotocol.NewAudioTranscriptionModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTranscriptionModel{protocol: model}, nil
}

func (m *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil AudioTranscriptionModel")
	}
	return m.protocol.Call(ctx, req)
}

type AudioTranslationModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	RequestOptions []option.RequestOption
}

func (c AudioTranslationModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranslationModelConfig) protocol() openaiprotocol.AudioTranslationModelConfig {
	return openaiprotocol.AudioTranslationModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type AudioTranslationModel struct {
	protocol *openaiprotocol.AudioTranslationModel
}

func NewAudioTranslationModel(config AudioTranslationModelConfig) (*AudioTranslationModel, error) {
	model, err := openaiprotocol.NewAudioTranslationModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTranslationModel{protocol: model}, nil
}

func (m *AudioTranslationModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil AudioTranslationModel")
	}
	return m.protocol.Call(ctx, req)
}

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	RequestOptions []option.RequestOption
}

func (c AudioTTSModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTTSModelConfig) protocol() openaiprotocol.AudioTTSModelConfig {
	return openaiprotocol.AudioTTSModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type AudioTTSModel struct{ protocol *openaiprotocol.AudioTTSModel }

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	model, err := openaiprotocol.NewAudioTTSModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{protocol: model}, nil
}

func (m *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil AudioTTSModel")
	}
	return m.protocol.Call(ctx, req)
}

func (m *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	if m == nil || m.protocol == nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, errors.New("openai: nil AudioTTSModel")) }
	}
	if err := req.Validate(); err != nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, err) }
	}
	return m.protocol.Stream(ctx, req)
}

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	RequestOptions []option.RequestOption
}

func (c ImageModelConfig) Validate() error { return c.protocol().Validate() }

func (c ImageModelConfig) protocol() openaiprotocol.ImageModelConfig {
	return openaiprotocol.ImageModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type ImageModel struct{ protocol *openaiprotocol.ImageModel }

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	model, err := openaiprotocol.NewImageModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &ImageModel{protocol: model}, nil
}

func (m *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil ImageModel")
	}
	return m.protocol.Call(ctx, req)
}

type ModerationModelConfig struct {
	APIKey         string
	DefaultOptions moderation.Options
	RequestOptions []option.RequestOption
}

func (c ModerationModelConfig) Validate() error { return c.protocol().Validate() }

func (c ModerationModelConfig) protocol() openaiprotocol.ModerationModelConfig {
	return openaiprotocol.ModerationModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, RequestOptions: c.RequestOptions}
}

type ModerationModel struct {
	protocol *openaiprotocol.ModerationModel
}

func NewModerationModel(config ModerationModelConfig) (*ModerationModel, error) {
	model, err := openaiprotocol.NewModerationModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &ModerationModel{protocol: model}, nil
}

func (m *ModerationModel) Call(ctx context.Context, req *moderation.Request) (*moderation.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("openai: nil ModerationModel")
	}
	return m.protocol.Call(ctx, req)
}
