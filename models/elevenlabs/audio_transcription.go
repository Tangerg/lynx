package elevenlabs

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions *transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("elevenlabs: APIKey is required")
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
	api            *API
	defaultOptions *transcription.Options
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:     cfg.APIKey,
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}
	apiReq, err := options.GetParams[TranscriptionRequest](mergedOpts.Extra, OptionsKey)
	if err != nil {
		return nil, err
	}
	if apiReq.ModelID == "" {
		apiReq.ModelID = mergedOpts.Model
	}
	if apiReq.LanguageCode == "" && mergedOpts.Language != "" {
		apiReq.LanguageCode = mergedOpts.Language
	}

	audio, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.Transcription(ctx, audio, req.Audio.MIME, apiReq)
	if err != nil {
		return nil, err
	}

	resultMeta := &transcription.ResultMetadata{}
	if apiResp.LanguageCode != "" {
		if err := resultMeta.Set("language_code", apiResp.LanguageCode); err != nil {
			return nil, err
		}
		if err := resultMeta.Set("language_probability", apiResp.LanguageProbability); err != nil {
			return nil, err
		}
	}
	if len(apiResp.Words) > 0 {
		if err := resultMeta.Set("words", apiResp.Words); err != nil {
			return nil, err
		}
	}

	result, err := transcription.NewResult(apiResp.Text, resultMeta)
	if err != nil {
		return nil, err
	}

	return transcription.NewResponse(result, &transcription.ResponseMetadata{})
}
