package deepgram

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("deepgram: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("deepgram: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps Deepgram's /v1/listen synchronous
// transcription endpoint. Supported models include "nova-3" (latest),
// "nova-2", "enhanced", "base". Diarization, smart_format, punctuation
// and the long tail of Deepgram knobs live on [ListenParams] and reach
// the API via the Extra-threaded SDK params, see [getOptionsParams].
//
// The returned [transcription.Result] holds the merged transcript of
// channel 0 / alternative 0; per-word + per-utterance breakdown is
// stashed on the result metadata so callers needing diarization or
// timestamps can dig in.
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
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[ListenParams](mergedOpts, OptionsKey)
	if params.Model == "" {
		params.Model = mergedOpts.Model
	}
	if params.Language == "" && mergedOpts.Language != "" {
		params.Language = mergedOpts.Language
	}

	audio, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}

	contentType := req.Audio.MIME

	apiResp, err := a.api.Listen(ctx, audio, contentType, params)
	if err != nil {
		return nil, err
	}

	if len(apiResp.Results.Channels) == 0 || len(apiResp.Results.Channels[0].Alternatives) == 0 {
		return nil, errors.New("deepgram: response has no transcript alternatives")
	}

	alt := apiResp.Results.Channels[0].Alternatives[0]

	resultMeta := &transcription.ResultMetadata{}
	resultMeta.Set("confidence", alt.Confidence)
	resultMeta.Set("words", alt.Words)
	if len(apiResp.Results.Utterances) > 0 {
		resultMeta.Set("utterances", apiResp.Results.Utterances)
	}

	result, err := transcription.NewResult(alt.Transcript, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	meta.Set("request_id", apiResp.Metadata.RequestID)
	meta.Set("duration", apiResp.Metadata.Duration)
	meta.Set("channels", apiResp.Metadata.Channels)

	return transcription.NewResponse(result, meta)
}

func (a *AudioTranscriptionModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranscriptionModel) Metadata() transcription.ModelMetadata {
	return transcription.ModelMetadata{
		Provider: Provider,
	}
}
