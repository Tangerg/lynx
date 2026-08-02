package google

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/image"
	"github.com/Tangerg/lynx/core/media"
	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/core/transcription"
	googleprotocol "github.com/Tangerg/lynx/models/internal/protocol/google"
	openaiprotocol "github.com/Tangerg/lynx/models/internal/protocol/openai"
)

const (
	Provider       = "Google"
	DefaultBaseURL = googleprotocol.DefaultBaseURL
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

	ModelGemini36Flash      = googleprotocol.ModelGemini36Flash
	ModelGemini35Flash      = googleprotocol.ModelGemini35Flash
	ModelGemini35FlashLite  = googleprotocol.ModelGemini35FlashLite
	ModelGemini31ProPreview = googleprotocol.ModelGemini31ProPreview

	ModelGemini25FlashPreviewTTS = googleprotocol.ModelGemini25FlashPreviewTTS
	ModelGemini25ProPreviewTTS   = googleprotocol.ModelGemini25ProPreviewTTS
	ModelGemini31FlashTTSPreview = googleprotocol.ModelGemini31FlashTTSPreview

	ModelGemini25FlashImage     = googleprotocol.ModelGemini25FlashImage
	ModelGemini3ProImage        = googleprotocol.ModelGemini3ProImage
	ModelGemini31FlashImage     = googleprotocol.ModelGemini31FlashImage
	ModelGemini31FlashLiteImage = googleprotocol.ModelGemini31FlashLiteImage

	ModelGeminiEmbedding2 = googleprotocol.ModelGeminiEmbedding2
)

type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error { return c.protocol().Validate() }

func (c ChatConfig) protocol() googleprotocol.ChatConfig {
	return googleprotocol.ChatConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, Backend: c.Backend,
		Project: c.Project, Location: c.Location, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type Chat struct{ protocol *googleprotocol.Chat }

func NewChat(config ChatConfig) (*Chat, error) {
	model, err := googleprotocol.NewChat(config.protocol())
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
	RequestOptions []option.RequestOption
}

type OpenAIChat struct{ protocol *openaiprotocol.Chat }

func NewOpenAIChat(config OpenAIChatConfig) (*OpenAIChat, error) {
	if config.APIKey == "" {
		return nil, errors.New("google: APIKey is required")
	}
	requestOptions := append([]option.RequestOption{option.WithBaseURL(cmp.Or(config.BaseURL, BaseURLOpenAI))}, config.RequestOptions...)
	model, err := openaiprotocol.NewCompatibleChat(
		openaiprotocol.ChatConfig{APIKey: config.APIKey, DefaultOptions: config.DefaultOptions, RequestOptions: requestOptions},
		openaiprotocol.Dialect{Provider: "google"},
	)
	if err != nil {
		return nil, fmt.Errorf("google: construct OpenAI-compatible chat: %w", err)
	}
	return &OpenAIChat{protocol: model}, nil
}

func (c *OpenAIChat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	if c == nil || c.protocol == nil {
		return nil, errors.New("google: nil OpenAIChat")
	}
	return c.protocol.Call(ctx, req)
}

func (c *OpenAIChat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	if c == nil || c.protocol == nil {
		return func(yield func(*corechat.Response, error) bool) { yield(nil, errors.New("google: nil OpenAIChat")) }
	}
	return c.protocol.Stream(ctx, req)
}

type EmbeddingModelConfig struct {
	APIKey         string
	DefaultOptions embedding.Options
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
	HTTPClient     *http.Client
}

func (c EmbeddingModelConfig) Validate() error { return c.protocol().Validate() }

func (c EmbeddingModelConfig) protocol() googleprotocol.EmbeddingModelConfig {
	return googleprotocol.EmbeddingModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, Backend: c.Backend,
		Project: c.Project, Location: c.Location, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type EmbeddingModel struct {
	protocol *googleprotocol.EmbeddingModel
}

func NewEmbeddingModel(config EmbeddingModelConfig) (*EmbeddingModel, error) {
	model, err := googleprotocol.NewEmbeddingModel(config.protocol())
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
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTTSModelConfig) protocol() googleprotocol.AudioTTSModelConfig {
	return googleprotocol.AudioTTSModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, Backend: c.Backend,
		Project: c.Project, Location: c.Location, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type AudioTTSModel struct{ protocol *googleprotocol.AudioTTSModel }

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	model, err := googleprotocol.NewAudioTTSModel(config.protocol())
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
	Backend        genai.Backend
	Project        string
	Location       string
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error { return c.protocol().Validate() }

func (c AudioTranscriptionModelConfig) protocol() googleprotocol.AudioTranscriptionModelConfig {
	return googleprotocol.AudioTranscriptionModelConfig{
		Provider: "google", APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, Backend: c.Backend,
		Project: c.Project, Location: c.Location, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type AudioTranscriptionModel struct {
	protocol *googleprotocol.AudioTranscriptionModel
}

func NewAudioTranscriptionModel(config AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	model, err := googleprotocol.NewAudioTranscriptionModel(config.protocol())
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

func (c ImageModelConfig) protocol() googleprotocol.ImageModelConfig {
	return googleprotocol.ImageModelConfig{APIKey: c.APIKey, DefaultOptions: c.DefaultOptions, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient}
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

type ImageModel struct{ protocol *googleprotocol.ImageModel }

func NewImageModel(config ImageModelConfig) (*ImageModel, error) {
	model, err := googleprotocol.NewImageModel(config.protocol())
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
	Backend    genai.Backend
	Project    string
	Location   string
	BaseURL    string
	HTTPClient *http.Client
}

func (c TextEstimatorConfig) Validate() error { return c.protocol().Validate() }

func (c TextEstimatorConfig) protocol() googleprotocol.TextEstimatorConfig {
	return googleprotocol.TextEstimatorConfig{
		APIKey: c.APIKey, Model: c.Model, Backend: c.Backend, Project: c.Project,
		Location: c.Location, BaseURL: c.BaseURL, HTTPClient: c.HTTPClient,
	}
}

type TextEstimator struct{ protocol *googleprotocol.TextEstimator }

func NewTextEstimator(config TextEstimatorConfig) (*TextEstimator, error) {
	estimator, err := googleprotocol.NewTextEstimator(config.protocol())
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
