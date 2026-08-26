package openai

import (
	"net/http"

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
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() openaiprotocol.ChatConfig {
	return openaiprotocol.ChatConfig{
		APIKey:         c.APIKey,
		DefaultOptions: c.DefaultOptions,
		BaseURL:        c.BaseURL,
		HTTPClient:     c.HTTPClient,
	}
}

// Chat is the OpenAI Chat Completions protocol model.
type Chat = openaiprotocol.Chat

func NewChat(config ChatConfig) (*Chat, error) {
	return openaiprotocol.NewChat(config.protocol())
}

// ResponsesChat is the OpenAI Responses API model.
type ResponsesChat = openaiprotocol.ResponsesChat

func NewResponsesChat(config ChatConfig) (*ResponsesChat, error) {
	return openaiprotocol.NewResponsesChat(config.protocol())
}

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error { return c.protocol().Validate() }

func (c EmbeddingModelConfig) protocol() openaiprotocol.EmbeddingModelConfig {
	return openaiprotocol.EmbeddingModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// EmbeddingModel is the OpenAI-compatible embedding protocol model.
type EmbeddingModel = openaiprotocol.EmbeddingModel

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	return openaiprotocol.NewEmbeddingModel(config.protocol())
}

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranscriptionModelConfig) protocol() openaiprotocol.AudioTranscriptionModelConfig {
	return openaiprotocol.AudioTranscriptionModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// AudioTranscriptionModel is the OpenAI-compatible transcription protocol model.
type AudioTranscriptionModel = openaiprotocol.AudioTranscriptionModel

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	return openaiprotocol.NewAudioTranscriptionModel(config.protocol())
}

type AudioTranslationModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranslationModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranslationModelConfig) protocol() openaiprotocol.AudioTranslationModelConfig {
	return openaiprotocol.AudioTranslationModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// AudioTranslationModel is the OpenAI-compatible translation protocol model.
type AudioTranslationModel = openaiprotocol.AudioTranslationModel

func NewAudioTranslationModel(config AudioTranslationModelConfig) (*AudioTranslationModel, error) {
	return openaiprotocol.NewAudioTranslationModel(config.protocol())
}

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTTSModelConfig) protocol() openaiprotocol.AudioTTSModelConfig {
	return openaiprotocol.AudioTTSModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// AudioTTSModel is the OpenAI-compatible speech protocol model.
type AudioTTSModel = openaiprotocol.AudioTTSModel

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	return openaiprotocol.NewAudioTTSModel(config.protocol())
}

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error { return c.protocol().Validate() }

func (c ImageModelConfig) protocol() openaiprotocol.ImageModelConfig {
	return openaiprotocol.ImageModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// ImageModel is the OpenAI-compatible image protocol model.
type ImageModel = openaiprotocol.ImageModel

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	return openaiprotocol.NewImageModel(config.protocol())
}

type ModerationModelConfig struct {
	APIKey         string
	DefaultOptions moderation.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ModerationModelConfig) Validate() error { return c.protocol().Validate() }

func (c ModerationModelConfig) protocol() openaiprotocol.ModerationModelConfig {
	return openaiprotocol.ModerationModelConfig{Provider: "openai", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

// ModerationModel is the OpenAI-compatible moderation protocol model.
type ModerationModel = openaiprotocol.ModerationModel

func NewModerationModel(config ModerationModelConfig) (*ModerationModel, error) {
	return openaiprotocol.NewModerationModel(config.protocol())
}
