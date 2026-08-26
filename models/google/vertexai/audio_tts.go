package vertexai

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"google.golang.org/genai"

	tts "github.com/Tangerg/lynx/core/speech"
	"github.com/Tangerg/lynx/models/google/internal/protocol"
)

type AudioTTSModelConfig struct {
	Project        string
	Location       string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (a AudioTTSModelConfig) Validate() error {
	if a.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if a.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel struct{ protocol *protocol.AudioTTSModel }

// NewAudioTTSModel returns a Vertex AI speech model.
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	adapter, err := protocol.NewAudioTTSModel(protocol.AudioTTSModelConfig{
		Provider:       "vertexai",
		Backend:        genai.BackendVertexAI,
		Project:        cfg.Project,
		Location:       cfg.Location,
		DefaultOptions: cfg.DefaultOptions,
		BaseURL:        cfg.BaseURL,
		HTTPClient:     cfg.HTTPClient,
	})
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
