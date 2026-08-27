package vertexai

import (
	"context"
	"errors"
	"iter"

	"google.golang.org/genai"

	tts "github.com/Tangerg/scope/core/speech"
	"github.com/Tangerg/scope/models/google/internal/protocol"
)

type AudioTTSModelConfig struct {
	Client         ClientConfig
	DefaultOptions tts.Options
}

func (a AudioTTSModelConfig) Validate() error {
	return a.Client.validateModelOptions(a.DefaultOptions.Model, a.DefaultOptions)
}

func (a AudioTTSModelConfig) protocol() protocol.AudioTTSModelConfig {
	return protocol.AudioTTSModelConfig{
		Provider:       protocolProvider,
		Backend:        genai.BackendVertexAI,
		Project:        a.Client.Project,
		Location:       a.Client.Location,
		DefaultOptions: a.DefaultOptions,
		BaseURL:        a.Client.BaseURL,
		HTTPClient:     a.Client.HTTPClient,
	}
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel struct{ protocol *protocol.AudioTTSModel }

func NewAudioTTSModel(config AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewAudioTTSModel(config.protocol())
	if err != nil {
		return nil, err
	}
	return &AudioTTSModel{protocol: adapter}, nil
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if a == nil || a.protocol == nil {
		return nil, errors.New("vertexai: nil AudioTTSModel")
	}
	return a.protocol.Call(ctx, req)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	if a == nil || a.protocol == nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, errors.New("vertexai: nil AudioTTSModel")) }
	}
	if err := req.Validate(); err != nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, err) }
	}
	return a.protocol.Stream(ctx, req)
}
