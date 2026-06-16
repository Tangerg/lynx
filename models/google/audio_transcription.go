package google

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/audio/transcription"
	"github.com/Tangerg/lynx/models/internal/options"
)

type AudioTranscriptionModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *transcription.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatModelConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	// Metadata overrides the [transcription.ModelMetadata] returned by
	// [AudioTranscriptionModel.Metadata]. Zero Provider falls back to [Provider].
	Metadata *transcription.ModelMetadata
}

func (c AudioTranscriptionModelConfig) Validate() error {
	if c.Backend != genai.BackendVertexAI && c.APIKey == nil {
		return errors.New("google: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

var _ transcription.Model = (*AudioTranscriptionModel)(nil)

// AudioTranscriptionModel exposes Gemini's multimodal chat through the
// transcription interface. Gemini has no /transcribe endpoint — any
// audio-accepting model returns a transcript when prompted. The default
// prompt is "Transcribe this audio."; callers override it by setting a
// string at Options.Extra[OptionsKeyTranscriptionPrompt].
type AudioTranscriptionModel struct {
	api            *API
	defaultOptions *transcription.Options
	metadata       transcription.ModelMetadata
}

func NewAudioTranscriptionModel(cfg AudioTranscriptionModelConfig) (*AudioTranscriptionModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:   cfg.APIKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
		BaseURL:  cfg.BaseURL,
	})
	if err != nil {
		return nil, err
	}

	info := transcription.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &AudioTranscriptionModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		metadata:       info,
	}, nil
}

// OptionsKeyTranscriptionPrompt selects the user-supplied prompt Gemini
// receives alongside the audio bytes. Stored on Options.Extra so the
// the transcription.Options stays minimal.
const OptionsKeyTranscriptionPrompt = "prompt"

func (a *AudioTranscriptionModel) buildAPITranscriptionRequest(req *transcription.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	mergedOpts, err := transcription.MergeOptions(a.defaultOptions, req.Options)
	if err != nil {
		return "", nil, nil, err
	}

	cfg := options.GetParams[genai.GenerateContentConfig](mergedOpts, OptionsKey)

	data, err := req.Audio.DataAsBytes()
	if err != nil {
		return "", nil, nil, err
	}

	prompt := "Transcribe this audio."
	if v, ok := mergedOpts.Get(OptionsKeyTranscriptionPrompt); ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			prompt = s
		}
	}

	parts := []*genai.Part{
		genai.NewPartFromText(prompt),
		genai.NewPartFromBytes(data, req.Audio.MimeType.TypeAndSubType()),
	}
	contents := []*genai.Content{
		genai.NewContentFromParts(parts, genai.RoleUser),
	}

	return mergedOpts.Model, contents, cfg, nil
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

	result, err := transcription.NewResult(text.String(), &transcription.ResultMetadata{})
	if err != nil {
		return nil, err
	}

	meta := &transcription.ResponseMetadata{Model: apiResp.ModelVersion}

	return transcription.NewResponse(result, meta)
}

func (a *AudioTranscriptionModel) Call(ctx context.Context, req *transcription.Request) (*transcription.Response, error) {
	modelName, contents, cfg, err := a.buildAPITranscriptionRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.ChatCompletion(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}

	return a.buildTranscriptionResponse(apiResp)
}

func (a *AudioTranscriptionModel) DefaultOptions() transcription.Options {
	return *a.defaultOptions
}

func (a *AudioTranscriptionModel) Metadata() transcription.ModelMetadata {
	return a.metadata
}
