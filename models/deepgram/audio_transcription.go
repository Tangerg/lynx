package deepgram

import (
	"context"
	"errors"
	"net/http"

	"github.com/Tangerg/lynx/core/transcription"
)

type AudioTranscriptionModelConfig struct {
	APIKey         string
	DefaultOptions transcription.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AudioTranscriptionModelConfig) Validate() error {
	if a.APIKey == "" {
		return errors.New("deepgram: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("deepgram: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
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
// The returned [transcription.Output] holds the merged transcript of
// channel 0 / alternative 0; per-word + per-utterance breakdown is
// stashed on the output metadata so callers needing diarization or
// timestamps can dig in.
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
	paramsValue, _, err := mergedOpts.Extensions.Decode[listenParams](TranscriptionRequestExtensionKey)
	params := &paramsValue
	if err != nil {
		return nil, err
	}
	if params.Model == "" {
		params.Model = mergedOpts.Model
	}
	if params.Language == "" && mergedOpts.Language != "" {
		params.Language = mergedOpts.Language
	}
	if params.Summarize == "v1" {
		return nil, errors.New("deepgram: summarize=v1 is deprecated; use true or v2")
	}

	audio, err := req.Audio.Bytes()
	if err != nil {
		return nil, err
	}

	contentType := req.Audio.MIME

	apiResp, err := a.api.listen(ctx, audio, contentType, params)
	if err != nil {
		return nil, err
	}

	if len(apiResp.Results.Channels) == 0 || len(apiResp.Results.Channels[0].Alternatives) == 0 {
		return nil, errors.New("deepgram: response has no transcript alternatives")
	}

	alt := apiResp.Results.Channels[0].Alternatives[0]

	outputMetadata := &transcription.OutputMetadata{}
	if setErr := outputMetadata.Set("deepgram/confidence", alt.Confidence); setErr != nil {
		return nil, setErr
	}
	if setErr := outputMetadata.Set("deepgram/words", alt.Words); setErr != nil {
		return nil, setErr
	}
	if len(apiResp.Results.Utterances) > 0 {
		if setErr := outputMetadata.Set("deepgram/utterances", apiResp.Results.Utterances); setErr != nil {
			return nil, setErr
		}
	}

	output, err := transcription.NewOutput(alt.Transcript, outputMetadata)
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{Model: params.Model}
	if err := meta.Set("deepgram/request_id", apiResp.Metadata.RequestID); err != nil {
		return nil, err
	}
	if err := meta.Set("deepgram/duration_seconds", apiResp.Metadata.Duration); err != nil {
		return nil, err
	}
	if err := meta.Set("deepgram/channels", apiResp.Metadata.Channels); err != nil {
		return nil, err
	}
	if err := meta.Set(TranscriptionResponseExtensionKey, apiResp.Raw); err != nil {
		return nil, err
	}

	return transcription.NewResponse(output, meta)
}
