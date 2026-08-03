package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/protocol/openai/internal/options"
)

type AudioTTSModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions tts.Options
	RequestOptions []option.RequestOption
}

func (c AudioTTSModelConfig) Validate() error {
	if err := validateProvider(c.Provider); err != nil {
		return fmt.Errorf("openai: Provider: %w", err)
	}
	if c.APIKey == "" {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("openai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

type AudioTTSModel struct {
	api            *API
	provider       string
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTTSModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (a *AudioTTSModel) buildAPITTSRequest(req *tts.Request) (*openai.AudioSpeechNewParams, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return nil, err
	}

	params, err := options.GetParams[openai.AudioSpeechNewParams](mergedOpts.Extensions, protocolModalityRequestExtensionKey(a.provider, "speech"))
	if err != nil {
		return nil, err
	}

	params.Model = mergedOpts.Model
	params.Input = req.Text
	// Each typed option only overrides extension-threaded params when set —
	// empty strings / zero speed would clobber prior choices, and
	// Speed=0 is outside the API's 0.25–4.0 range.
	if mergedOpts.Voice != "" {
		params.Voice = openai.AudioSpeechNewParamsVoiceUnion{OfString: param.NewOpt(mergedOpts.Voice)}
	}
	if mergedOpts.Speed != 0 {
		params.Speed = openai.Float(mergedOpts.Speed)
	}
	if mergedOpts.OutputFormat != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(mergedOpts.OutputFormat)
	}
	params.StreamFormat = openai.AudioSpeechNewParamsStreamFormatAudio

	return params, nil
}

func (a *AudioTTSModel) buildTTSResponse(data []byte) (*tts.Response, error) {
	result, err := tts.NewResult(data, &tts.ResultMetadata{})
	if err != nil {
		return nil, err
	}
	return tts.NewResponse(result, &tts.ResponseMetadata{})
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	apiReq, err := a.buildAPITTSRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.AudioTTS(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	defer apiResp.Body.Close()

	data, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return nil, err
	}

	return a.buildTTSResponse(data)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		apiReq, err := a.buildAPITTSRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiResp, err := a.api.AudioTTS(ctx, apiReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer apiResp.Body.Close()

		for chunk, err := range readAudioChunks(apiResp.Body) {
			if err != nil {
				yield(nil, err)
				return
			}

			resp, err := a.buildTTSResponse(chunk)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}
	}
}

func readAudioChunks(reader io.Reader) iter.Seq2[[]byte, error] {
	const chunkSize = 16 * 1024
	return func(yield func([]byte, error) bool) {
		for {
			buffer := make([]byte, chunkSize)
			read, err := reader.Read(buffer)
			eof := err == io.EOF
			if eof {
				err = nil
			}
			if read > 0 || err != nil {
				if !yield(buffer[:read], err) {
					return
				}
			}
			if eof || err != nil {
				return
			}
		}
	}
}
