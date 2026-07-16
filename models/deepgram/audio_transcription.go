package deepgram

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("deepgram: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("deepgram: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel wraps Deepgram's /v1/listen synchronous
// transcription endpoint. Supported models include "nova-3" (latest),
// "nova-2", "enhanced", "base". Diarization, smart_format, punctuation
// and the long tail of Deepgram knobs live on [ListenParams] and reach
// the API via the extension-threaded SDK params, see [getOptionsParams].
//
// The returned [transcription.Result] holds the merged transcript of
// channel 0 / alternative 0; per-word + per-utterance breakdown is
// stashed on the result metadata so callers needing diarization or
// timestamps can dig in.
type AudioTranscriptionModel struct {
	api            *API
	defaultOptions transcription.Options
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
	params, err := options.GetParams[ListenParams](mergedOpts.Extensions, OptionsKey)
	if err != nil {
		return nil, err
	}
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
	if err := resultMeta.Set("confidence", alt.Confidence); err != nil {
		return nil, err
	}
	if err := resultMeta.Set("words", alt.Words); err != nil {
		return nil, err
	}
	if len(apiResp.Results.Utterances) > 0 {
		if err := resultMeta.Set("utterances", apiResp.Results.Utterances); err != nil {
			return nil, err
		}
	}

	result, err := transcription.NewResult(alt.Transcript, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{}
	if err := meta.Set("request_id", apiResp.Metadata.RequestID); err != nil {
		return nil, err
	}
	if err := meta.Set("duration", apiResp.Metadata.Duration); err != nil {
		return nil, err
	}
	if err := meta.Set("channels", apiResp.Metadata.Channels); err != nil {
		return nil, err
	}

	return transcription.NewResponse(result, meta)
}
