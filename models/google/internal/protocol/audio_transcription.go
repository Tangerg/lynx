package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/transcription"
)

type AudioTranscriptionModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions transcription.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	HTTPClient *http.Client
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if err := validateProvider(c.Provider); err != nil {
		return fmt.Errorf("google: Provider: %w", err)
	}
	if c.Backend != genai.BackendVertexAI && c.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions.Model == "" {
		return errors.New("google: DefaultOptions.Model is required")
	}
	if _, err := c.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel exposes Gemini's multimodal chat through the
// transcription interface. Gemini has no /transcribe endpoint — any
// audio-accepting model returns a transcript when prompted. This adapter uses
// the stable instruction "Transcribe this audio.".
type AudioTranscriptionModel struct {
	api            *api
	provider       string
	defaultOptions transcription.Options
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := newAPI(apiConfig{
		APIKey:     cfg.APIKey,
		Backend:    cfg.Backend,
		Project:    cfg.Project,
		Location:   cfg.Location,
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &AudioTranscriptionModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (a *AudioTranscriptionModel) buildAPITranscriptionRequest(req *transcription.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", nil, nil, err
	}
	if err := a.validateOptions(mergedOpts); err != nil {
		return "", nil, nil, err
	}

	cfgValue, _, err := mergedOpts.Extensions.Decode[genai.GenerateContentConfig](protocolKey(a.provider, "transcription_request"))

	cfg := &cfgValue
	if err != nil {
		return "", nil, nil, err
	}

	data, err := req.Audio.Bytes()
	if err != nil {
		return "", nil, nil, err
	}

	parts := []*genai.Part{
		genai.NewPartFromText("Transcribe this audio."),
		genai.NewPartFromBytes(data, req.Audio.MIME),
	}
	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	return mergedOpts.Model, contents, cfg, nil
}

func (*AudioTranscriptionModel) validateOptions(options transcription.Options) error {
	if options.Language != "" {
		return errors.New("google: transcription: language is not supported")
	}
	return nil
}

func (a *AudioTranscriptionModel) buildTranscriptionResponse(apiResp *genai.GenerateContentResponse) (*transcription.Response, error) {
	if len(apiResp.Candidates) == 0 || apiResp.Candidates[0].Content == nil {
		return nil, errors.New("google: transcription response has no candidates")
	}

	var text strings.Builder
	for _, part := range apiResp.Candidates[0].Content.Parts {
		// Skip thought-flagged parts: Gemini 2.5 may emit reasoning
		// before producing the transcript; reasoning text must not be
		// pasted into the output.
		if part.Thought {
			continue
		}
		if part.Text != "" {
			text.WriteString(part.Text)
		}
	}

	output, err := transcription.NewOutput(text.String(), &transcription.OutputMetadata{})
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{Model: apiResp.ModelVersion}
	if err := meta.Set(protocolKey(a.provider, "transcription_response"), apiResp); err != nil {
		return nil, err
	}

	return transcription.NewResponse(output, meta)
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	modelName, contents, cfg, err := a.buildAPITranscriptionRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.chatCompletion(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}

	return a.buildTranscriptionResponse(apiResp)
}
