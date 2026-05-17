package elevenlabs

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c *AudioTranscriptionModelConfig) validate() error {
	if c == nil {
		return errors.New("elevenlabs: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("elevenlabs: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("elevenlabs: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps ElevenLabs' /v1/speech-to-text endpoint
// (Scribe model family). Diarization / language / per-word timestamps
// are reached through the Extra-threaded [TranscriptionRequest].
type AudioTranscriptionModel struct {
	api            *Api
	defaultOptions *transcription.Options
}

func NewAudioTranscriptionModel(cfg *AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:     cfg.ApiKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[TranscriptionRequest](mergedOpts, OptionsKey)
	if apiReq.ModelID == "" {
		apiReq.ModelID = mergedOpts.Model
	}
	if apiReq.LanguageCode == "" && mergedOpts.Language != "" {
		apiReq.LanguageCode = mergedOpts.Language
	}

	audio, err := req.Audio.DataAsBytes()
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.Transcription(ctx, audio, req.Audio.MimeType.TypeAndSubType(), apiReq)
	if err != nil {
		return nil, err
	}

	resultMeta := &transcription.ResultMetadata{}
	if apiResp.LanguageCode != "" {
		resultMeta.Set("language_code", apiResp.LanguageCode)
		resultMeta.Set("language_probability", apiResp.LanguageProbability)
	}
	if len(apiResp.Words) > 0 {
		resultMeta.Set("words", apiResp.Words)
	}

	result, err := transcription.NewResult(apiResp.Text, resultMeta)
	if err != nil {
		return nil, err
	}

	return transcription.NewResponse(result, &transcription.ResponseMetadata{})
}

func (a *AudioTranscriptionModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranscriptionModel) Metadata() transcription.ModelMetadata {
	return transcription.ModelMetadata{Provider: Provider}
}
