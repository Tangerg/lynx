package protocol

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"slices"

	"google.golang.org/genai"

	tts "github.com/Tangerg/lynx/core/speech"
)

type AudioTTSModelConfig struct {
	Provider       string
	APIKey         string
	DefaultOptions tts.Options

	// Backend / Project / Location enable Vertex AI access — see
	// the matching fields on [ChatConfig] for semantics.
	Backend  genai.Backend
	Project  string
	Location string

	// BaseURL overrides the genai endpoint. Optional.
	BaseURL string

	HTTPClient *http.Client
}

func (a AudioTTSModelConfig) Validate() error {
	if err := validateProvider(a.Provider); err != nil {
		return fmt.Errorf("google: Provider: %w", err)
	}
	if a.Backend != genai.BackendVertexAI && a.APIKey == "" {
		return errors.New("google: APIKey is required")
	}
	if a.DefaultOptions.Model == "" {
		return errors.New("google: DefaultOptions.Model is required")
	}
	if _, err := a.DefaultOptions.Merged(); err != nil {
		return err
	}
	return nil
}

var _ tts.Model = (*AudioTTSModel)(nil)
var _ tts.Streamer = (*AudioTTSModel)(nil)

// AudioTTSModel wraps Gemini's native TTS through GenerateContent with
// ResponseModalities=AUDIO. Current supported models are declared in
// constant.go; only Gemini 3.1 Flash TTS supports incremental streaming.
//
// Speed and OutputFormat are not honored: Gemini's TTS has no
// playback-rate knob. GenerateContent returns 24 kHz signed 16-bit
// little-endian PCM; callers choose their own container at the application
// boundary.
type AudioTTSModel struct {
	api            *api
	provider       string
	defaultOptions tts.Options
}

func NewAudioTTSModel(cfg AudioTTSModelConfig) (*AudioTTSModel, error) {
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

	return &AudioTTSModel{
		api:            api,
		provider:       cfg.Provider,
		defaultOptions: cfg.DefaultOptions.Clone(),
	}, nil
}

func (a *AudioTTSModel) buildAPITTSRequest(req *tts.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	mergedOpts, err := a.defaultOptions.Merged(req.Options)
	if err != nil {
		return "", nil, nil, err
	}
	if err := a.validateOptions(mergedOpts); err != nil {
		return "", nil, nil, err
	}

	cfgValue, _, err := mergedOpts.Extensions.Decode[genai.GenerateContentConfig](protocolKey(a.provider, "speech_request"))

	cfg := &cfgValue
	if err != nil {
		return "", nil, nil, err
	}

	// Force AUDIO output. The caller may have set ResponseModalities via
	// Extra (e.g. ["AUDIO", "TEXT"] for hybrid response); preserve that
	// when it already includes AUDIO, otherwise overwrite.
	if !slices.Contains(cfg.ResponseModalities, string(genai.ModalityAudio)) {
		cfg.ResponseModalities = []string{string(genai.ModalityAudio)}
	}

	// Voice routes onto SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName.
	// If the caller already threaded a richer SpeechConfig through Extra
	// (multi-speaker dialog, language code, replicated voice) it is
	// kept; the prebuilt-voice slot is only filled when the caller
	// did not supply one.
	if mergedOpts.Voice != "" {
		if cfg.SpeechConfig == nil {
			cfg.SpeechConfig = &genai.SpeechConfig{}
		}
		if cfg.SpeechConfig.VoiceConfig == nil && cfg.SpeechConfig.MultiSpeakerVoiceConfig == nil {
			cfg.SpeechConfig.VoiceConfig = &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: mergedOpts.Voice,
				},
			}
		}
	}

	contents := []*genai.Content{
		genai.NewContentFromText(req.Text, genai.RoleUser),
	}

	return mergedOpts.Model, contents, cfg, nil
}

func (*AudioTTSModel) validateOptions(options tts.Options) error {
	switch {
	case options.OutputFormat != "":
		return errors.New("google: speech: output_format is not supported")
	case options.Speed != 0:
		return errors.New("google: speech: speed is not supported")
	default:
		return nil
	}
}

// errNoAudio signals "this chunk contained no audio Parts". Returned by
// buildTTSResponse so the streaming loop can skip such chunks without
// terminating the whole stream.
var errNoAudio = errors.New("google: tts chunk has no audio inline-data parts")

func (a *AudioTTSModel) buildTTSResponse(apiResp *genai.GenerateContentResponse) (*tts.Response, error) {
	if len(apiResp.Candidates) == 0 || apiResp.Candidates[0].Content == nil {
		return nil, errNoAudio
	}

	// Capture mime type from the first audio-bearing Part — preceding
	// Parts may be thought / metadata with nil InlineData.
	var (
		audio    []byte
		mimeType string
	)
	for _, part := range apiResp.Candidates[0].Content.Parts {
		if part.InlineData == nil || len(part.InlineData.Data) == 0 {
			continue
		}
		if mimeType == "" {
			mimeType = part.InlineData.MIMEType
		}
		audio = append(audio, part.InlineData.Data...)
	}
	if len(audio) == 0 {
		return nil, errNoAudio
	}

	outputMetadata := &tts.OutputMetadata{}
	if mimeType != "" {
		if err := outputMetadata.Set(protocolKey(a.provider, "mime_type"), mimeType); err != nil {
			return nil, err
		}
	}

	output, err := tts.NewOutput(audio, outputMetadata)
	if err != nil {
		return nil, err
	}

	meta := &tts.ResponseMetadata{Model: apiResp.ModelVersion}
	if err := meta.Set(protocolKey(a.provider, "speech_response"), apiResp); err != nil {
		return nil, err
	}

	return tts.NewResponse(output, meta)
}

func (a *AudioTTSModel) Call(ctx context.Context, req *tts.Request) (*tts.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	modelName, contents, cfg, err := a.buildAPITTSRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := a.api.chatCompletion(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}

	return a.buildTTSResponse(apiResp)
}

func (a *AudioTTSModel) Stream(ctx context.Context, req *tts.Request) iter.Seq2[*tts.Response, error] {
	return func(yield func(*tts.Response, error) bool) {
		if err := req.Validate(); err != nil {
			yield(nil, err)
			return
		}
		modelName, contents, cfg, err := a.buildAPITTSRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		if modelName != ModelGemini31FlashTTSPreview {
			yield(nil, fmt.Errorf("google: speech: model %q does not support streaming; use %q", modelName, ModelGemini31FlashTTSPreview))
			return
		}

		for chunk, err := range a.api.chatCompletionStream(ctx, modelName, contents, cfg) {
			if err != nil {
				yield(nil, err)
				return
			}

			resp, err := a.buildTTSResponse(chunk)
			if err != nil {
				// Skip chunks that don't carry audio (Gemini may emit
				// metadata-only chunks during streaming) rather than
				// fail the whole stream.
				if errors.Is(err, errNoAudio) {
					continue
				}
				yield(nil, err)
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}
