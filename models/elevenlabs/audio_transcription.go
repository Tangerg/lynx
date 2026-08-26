package elevenlabs

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Tangerg/lynx/core/transcription"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("elevenlabs: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("elevenlabs: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps ElevenLabs' /v1/speech-to-text endpoint
// (Scribe model family). Diarization / language / per-word timestamps
// are reached through the extension-threaded [TranscriptionRequest].
type AudioTranscriptionModel struct {
	api            *api
	defaultOptions transcription.Options
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions.Clone(),
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
	apiReqValue, _, err := mergedOpts.Extensions.Decode[transcriptionRequest](TranscriptionRequestExtensionKey)
	apiReq := &apiReqValue
	if err != nil {
		return nil, err
	}
	if apiReq.ModelID == "" {
		apiReq.ModelID = mergedOpts.Model
	}
	if apiReq.LanguageCode == "" && mergedOpts.Language != "" {
		apiReq.LanguageCode = mergedOpts.Language
	}
	if err := validateTranscriptionRequest(apiReq); err != nil {
		return nil, err
	}

	audio, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.transcription(ctx, audio, req.Audio.MIME, apiReq)
	if err != nil {
		return nil, err
	}

	outputMetadata := &transcription.OutputMetadata{}
	if apiResp.LanguageCode != "" {
		if err := outputMetadata.Set("elevenlabs/language_code", apiResp.LanguageCode); err != nil {
			return nil, err
		}
		if err := outputMetadata.Set("elevenlabs/language_probability", apiResp.LanguageProbability); err != nil {
			return nil, err
		}
	}
	if len(apiResp.Words) > 0 {
		if err := outputMetadata.Set("elevenlabs/words", apiResp.Words); err != nil {
			return nil, err
		}
	}
	if len(apiResp.Entities) > 0 {
		if err := outputMetadata.Set("elevenlabs/entities", apiResp.Entities); err != nil {
			return nil, err
		}
	}

	output, err := transcription.NewOutput(apiResp.Text, outputMetadata)
	if err != nil {
		return nil, err
	}

	responseMetadata := &transcription.ResponseMetadata{Model: apiReq.ModelID}
	if err := responseMetadata.Set(TranscriptionResponseExtensionKey, apiResp); err != nil {
		return nil, err
	}
	return transcription.NewResponse(output, responseMetadata)
}

func validateTranscriptionRequest(req *transcriptionRequest) error {
	if req.ModelID != ModelScribeV2 && req.ModelID != ModelScribeV1 {
		return fmt.Errorf("elevenlabs: transcription model must be %q or %q, got %q", ModelScribeV2, ModelScribeV1, req.ModelID)
	}
	if req.NumSpeakers != nil && (*req.NumSpeakers < 1 || *req.NumSpeakers > 32) {
		return fmt.Errorf("elevenlabs: num_speakers must be between 1 and 32, got %d", *req.NumSpeakers)
	}
	if req.DiarizationThreshold != nil && (*req.DiarizationThreshold < 0.1 || *req.DiarizationThreshold > 0.4) {
		return fmt.Errorf("elevenlabs: diarization_threshold must be between 0.1 and 0.4, got %g", *req.DiarizationThreshold)
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2) {
		return fmt.Errorf("elevenlabs: transcription temperature must be between 0 and 2, got %g", *req.Temperature)
	}
	if req.Seed != nil && *req.Seed < 0 {
		return fmt.Errorf("elevenlabs: transcription seed must be non-negative, got %d", *req.Seed)
	}
	if req.TimestampsGranularity != "" && req.TimestampsGranularity != "none" && req.TimestampsGranularity != "word" && req.TimestampsGranularity != "character" {
		return fmt.Errorf("elevenlabs: timestamps_granularity must be none, word, or character, got %q", req.TimestampsGranularity)
	}
	if req.FileFormat != "" && req.FileFormat != "other" && req.FileFormat != "pcm_s16le_16" {
		return fmt.Errorf("elevenlabs: file_format must be other or pcm_s16le_16, got %q", req.FileFormat)
	}
	if len(req.Keyterms) > 1000 {
		return fmt.Errorf("elevenlabs: keyterms must contain at most 1000 entries, got %d", len(req.Keyterms))
	}
	if req.ModelID == ModelScribeV1 && len(req.Keyterms) > 0 {
		return errors.New("elevenlabs: keyterms are only supported by scribe_v2")
	}
	return nil
}
