package google

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/google/internal/protocol"
	openaiprotocol "github.com/Tangerg/lynx/models/protocol/openai"
)

const (
	Provider       = "Google"
	DefaultBaseURL = protocol.DefaultBaseURL
	BaseURLOpenAI  = "https://generativelanguage.googleapis.com/v1beta/openai"

	RequestExtensionKey  = "google/request"
	ResponseExtensionKey = "google/response"

	SpeechRequestExtensionKey         = "google/speech_request"
	SpeechResponseExtensionKey        = "google/speech_response"
	TranscriptionRequestExtensionKey  = "google/transcription_request"
	TranscriptionResponseExtensionKey = "google/transcription_response"
	EmbeddingRequestExtensionKey      = "google/embedding_request"
	EmbeddingResponseExtensionKey     = "google/embedding_response"
	ImageRequestExtensionKey          = "google/image_request"
	ImageResponseExtensionKey         = "google/image_response"

	OpenAIRequestExtensionKey     = "google/openai_request"
	OpenAIResponseExtensionKey    = "google/openai_response"
	OpenAIStreamChunkExtensionKey = "google/openai_stream_chunk"

	ModelGemini36Flash      = protocol.ModelGemini36Flash
	ModelGemini35Flash      = protocol.ModelGemini35Flash
	ModelGemini35FlashLite  = protocol.ModelGemini35FlashLite
	ModelGemini31ProPreview = protocol.ModelGemini31ProPreview

	ModelGemini25FlashPreviewTTS = protocol.ModelGemini25FlashPreviewTTS
	ModelGemini25ProPreviewTTS   = protocol.ModelGemini25ProPreviewTTS
	ModelGemini31FlashTTSPreview = protocol.ModelGemini31FlashTTSPreview

	ModelGemini25FlashImage     = protocol.ModelGemini25FlashImage
	ModelGemini3ProImage        = protocol.ModelGemini3ProImage
	ModelGemini31FlashImage     = protocol.ModelGemini31FlashImage
	ModelGemini31FlashLiteImage = protocol.ModelGemini31FlashLiteImage

	ModelGeminiEmbedding2 = protocol.ModelGeminiEmbedding2
)

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() protocol.ChatConfig {
	return protocol.ChatConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions,
		BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type Chat struct{ protocol *protocol.Chat }

func NewChat(config ChatConfig) (*Chat, error) {
	model, err := protocol.NewChat(config.protocol())
	if err != nil {
		return nil, err
	}
	return &Chat{protocol: model}, nil
}

func (c *Chat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("google: nil Chat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *Chat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("google: nil Chat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type OpenAIChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (config OpenAIChatConfig) Validate() error {
	if config.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if err := config.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("google: DefaultOptions: %w", err)
	}
	return nil
}

// OpenAIChat is Google's OpenAI-compatible protocol model.
type OpenAIChat = openaiprotocol.Chat

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return openaiprotocol.NewCompatibleChat(
		openaiprotocol.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, BaseURL: cmp.Or(config.BaseURL, BaseURLOpenAI), HTTPClient: config.HTTPClient},
		openaiprotocol.Dialect{Provider: "google", TokenLimitField: openaiprotocol.TokenLimitMaxTokens},
	)
}

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error { return c.protocol().Validate() }

func (c EmbeddingModelConfig) protocol() protocol.EmbeddingModelConfig {
	return protocol.EmbeddingModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions,
		BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type EmbeddingModel struct {
	protocol *protocol.EmbeddingModel
}

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	model, err := protocol.NewEmbeddingModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &EmbeddingModel{protocol: model}, nil
}

func (m *EmbeddingModel) Call(ctx context.Context, req *embedding.Request) (*embedding.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("google: nil EmbeddingModel")
	}
	return m.protocol.Call(ctx, req)
}

type AudioTTSModelConfig struct {
	APIKey         string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTTSModelConfig) protocol() protocol.AudioTTSModelConfig {
	return protocol.AudioTTSModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions,
		BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type AudioTTSModel struct{ protocol *protocol.AudioTTSModel }

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	model, err := protocol.NewAudioTTSModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{protocol: model}, nil
}

func (m *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("google: nil AudioTTSModel")
	}
	return m.protocol.Call(ctx, req)
}

func (m *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	if m == nil || m.protocol == nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, errors.New("google: nil AudioTTSModel")) }
	}
	if err := req.Validate(); err != nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, err) }
	}
	return m.protocol.Stream(ctx, req)
}

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranscriptionModelConfig) protocol() protocol.AudioTranscriptionModelConfig {
	return protocol.AudioTranscriptionModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions,
		BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type AudioTranscriptionModel struct {
	protocol *protocol.AudioTranscriptionModel
}

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	model, err := protocol.NewAudioTranscriptionModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTranscriptionModel{protocol: model}, nil
}

func (m *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("google: nil AudioTranscriptionModel")
	}
	return m.protocol.Call(ctx, req)
}

type ImageModelConfig struct {
	APIKey         string
	DefaultOptions image.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ImageModelConfig) Validate() error { return c.protocol().Validate() }

func (c ImageModelConfig) protocol() protocol.ImageModelConfig {
	return protocol.ImageModelConfig{APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
}

type ImageGenerationOptions struct {
	AspectRatio           string                    `json:"aspect_ratio,omitempty"`
	ImageSize             string                    `json:"image_size,omitempty"`
	Delivery              string                    `json:"delivery,omitempty"`
	PreviousInteractionID string                    `json:"previous_interaction_id,omitempty"`
	Store                 *bool                     `json:"store,omitempty"`
	ThinkingLevel         string                    `json:"thinking_level,omitempty"`
	ThinkingSummaries     string                    `json:"thinking_summaries,omitempty"`
	ServiceTier           string                    `json:"service_tier,omitempty"`
	Labels                map[string]string         `json:"labels,omitempty"`
	InputImages           []*media.Media            `json:"input_images,omitempty"`
	GoogleSearch          *ImageGoogleSearchOptions `json:"google_search,omitempty"`
	SafetySettings        []ImageSafetySetting      `json:"safety_settings,omitempty"`
}

type ImageGoogleSearchOptions struct {
	SearchTypes []string `json:"search_types,omitempty"`
}

type ImageSafetySetting struct {
	Type      string `json:"type"`
	Threshold string `json:"threshold"`
	Method    string `json:"method,omitempty"`
}

type ImageModel struct{ protocol *protocol.ImageModel }

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	model, err := protocol.NewImageModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &ImageModel{protocol: model}, nil
}

func (m *ImageModel) Call(ctx context.Context, req *image.Request) (*image.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("google: nil ImageModel")
	}
	return m.protocol.Call(ctx, req)
}

type TextEstimatorConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
}

func (c TextEstimatorConfig) Validate() error { return c.protocol().Validate() }

func (c TextEstimatorConfig) protocol() protocol.TextEstimatorConfig {
	return protocol.TextEstimatorConfig{
		APIKey: c.APIKey, Model: c.Model, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type TextEstimator struct{ protocol *protocol.TextEstimator }

func NewTextEstimator(config TextEstimatorConfig) (*TextEstimator, error) {
	estimator, err := protocol.NewTextEstimator(config.protocol())
	if err != nil {
		return nil, err
	}
	return &TextEstimator{protocol: estimator}, nil
}

func (e *TextEstimator) EstimateText(ctx context.Context, value string) (int, error) {
	if e == nil || e.protocol == nil {
		return 0, errors.New("google: nil TextEstimator")
	}
	return e.protocol.EstimateText(ctx, value)
}
