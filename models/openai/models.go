package openai

import (
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/image"
	"github.com/Tangerg/scope/core/moderation"
	tts "github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/core/transcription"
	openaiprotocol "github.com/Tangerg/scope/models/protocol/openai"
)

const (
	Provider         = "OpenAI"
	protocolProvider = "openai"

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

func (e EmbeddingModelConfig) Validate() error { return e.protocol().Validate() }

func (e EmbeddingModelConfig) protocol() openaiprotocol.EmbeddingModelConfig {
	return openaiprotocol.EmbeddingModelConfig{Provider: protocolProvider, APIKey: e.APIKey, DefaultOptions: e.DefaultOptions, BaseURL: e.BaseURL, HTTPClient: e.HTTPClient}
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

func (a AudioTranscriptionModelConfig) Validate() error { return a.protocol().Validate() }

func (a AudioTranscriptionModelConfig) protocol() openaiprotocol.AudioTranscriptionModelConfig {
	return openaiprotocol.AudioTranscriptionModelConfig{Provider: protocolProvider, APIKey: a.APIKey, DefaultOptions: a.DefaultOptions, BaseURL: a.BaseURL, HTTPClient: a.HTTPClient}
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

func (a AudioTranslationModelConfig) Validate() error { return a.protocol().Validate() }

func (a AudioTranslationModelConfig) protocol() openaiprotocol.AudioTranslationModelConfig {
	return openaiprotocol.AudioTranslationModelConfig{Provider: protocolProvider, APIKey: a.APIKey, DefaultOptions: a.DefaultOptions, BaseURL: a.BaseURL, HTTPClient: a.HTTPClient}
}

// AudioTranslationModel is the OpenAI-compatible translation protocol model.
type AudioTranslationModel = openaiprotocol.AudioTranslationModel

func NewAudioTranslationModel(config AudioTranslationModelConfig) (*AudioTranslationModel, error) {
	return openaiprotocol.NewAudioTranslationModel(config.protocol())
}

type AudioTTSModelConfig struct {
	APIKey           string
	DefaultOptions   tts.Options
	BaseURL          string
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

func (a AudioTTSModelConfig) Validate() error { return a.protocol().Validate() }

func (a AudioTTSModelConfig) protocol() openaiprotocol.AudioTTSModelConfig {
	return openaiprotocol.AudioTTSModelConfig{
		Provider:         protocolProvider,
		APIKey:           a.APIKey,
		DefaultOptions:   a.DefaultOptions,
		BaseURL:          a.BaseURL,
		HTTPClient:       a.HTTPClient,
		MaxResponseBytes: a.MaxResponseBytes,
	}
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

func (i ImageModelConfig) Validate() error { return i.protocol().Validate() }

func (i ImageModelConfig) protocol() openaiprotocol.ImageModelConfig {
	return openaiprotocol.ImageModelConfig{Provider: protocolProvider, APIKey: i.APIKey, DefaultOptions: i.DefaultOptions, BaseURL: i.BaseURL, HTTPClient: i.HTTPClient}
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

func (m ModerationModelConfig) Validate() error { return m.protocol().Validate() }

func (m ModerationModelConfig) protocol() openaiprotocol.ModerationModelConfig {
	return openaiprotocol.ModerationModelConfig{Provider: protocolProvider, APIKey: m.APIKey, DefaultOptions: m.DefaultOptions, BaseURL: m.BaseURL, HTTPClient: m.HTTPClient}
}

// ModerationModel is the OpenAI-compatible moderation protocol model.
type ModerationModel = openaiprotocol.ModerationModel

func NewModerationModel(config ModerationModelConfig) (*ModerationModel, error) {
	return openaiprotocol.NewModerationModel(config.protocol())
}
