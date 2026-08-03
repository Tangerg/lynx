package vertexai

import (
	"context"
	"errors"
	"iter"
	"net/http"

	"google.golang.org/genai"

	tts "github.com/Tangerg/lynx/core/speech"
	google "github.com/Tangerg/lynx/models/google/internal/protocol"
)

type AudioTTSModelConfig struct {
	Project        string
	Location       string
	DefaultOptions tts.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c AudioTTSModelConfig) Validate() error {
	if c.Project == "" {
		return errors.New("vertexai: Project is required")
	}
	if c.Location == "" {
		return errors.New("vertexai: Location is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("vertexai: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var (
	_ tts.Model    = (*AudioTTSModel)(nil)
	_ tts.Streamer = (*AudioTTSModel)(nil)
)

type AudioTTSModel struct{ protocol *google.AudioTTSModel }

// NewAudioTTSModel returns a Vertex AI speech model.
func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	protocol, err := google.NewAudioTTSModel(google.AudioTTSModelConfig{
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
	return &AudioTTSModel{protocol: protocol}, nil
}

func (m *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if m == nil || m.protocol == nil {
		return nil, errors.New("vertexai: nil AudioTTSModel")
	}
	return m.protocol.Call(ctx, req)
}

func (m *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	if m == nil || m.protocol == nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, errors.New("vertexai: nil AudioTTSModel")) }
	}
	if err := req.Validate(); err != nil {
		return func(yield func(*tts.Response, error) bool) { yield(nil, err) }
	}
	return m.protocol.Stream(ctx, req)
}
