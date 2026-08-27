package elevenlabs

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-resty/resty/v2"
)

type apiConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (a apiConfig) validate() error {
	if a.APIKey == "" {
		return errors.New("elevenlabs: APIKey is required")
	}
	return nil
}

type api struct {
	http *resty.Client
}

func newAPI(cfg apiConfig) (*api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client := resty.New()
	if cfg.HTTPClient != nil {
		client = resty.NewWithClient(cfg.HTTPClient)
	}
	client.SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetHeader("xi-api-key", cfg.APIKey).
		SetHeader("Accept", "audio/*")

	return &api{http: client}, nil
}

type ttsRequest struct {
	EnableLogging                   *bool                            `json:"-"`
	OptimizeStreamingLatency        *int                             `json:"-"`
	Text                            string                           `json:"text"`
	ModelID                         string                           `json:"model_id,omitempty"`
	LanguageCode                    string                           `json:"language_code,omitempty"`
	VoiceSettings                   *voiceSettings                   `json:"voice_settings,omitempty"`
	Seed                            *int64                           `json:"seed,omitempty"`
	PreviousText                    string                           `json:"previous_text,omitempty"`
	NextText                        string                           `json:"next_text,omitempty"`
	PreviousRequestIDs              []string                         `json:"previous_request_ids,omitzero"`
	NextRequestIDs                  []string                         `json:"next_request_ids,omitzero"`
	ApplyTextNormalization          string                           `json:"apply_text_normalization,omitempty"`
	ApplyLanguageTextNormalization  *bool                            `json:"apply_language_text_normalization,omitempty"`
	PronunciationDictionaryLocators []pronunciationDictionaryLocator `json:"pronunciation_dictionary_locators,omitzero"`
}

type pronunciationDictionaryLocator struct {
	PronunciationDictionaryID string `json:"pronunciation_dictionary_id"`
	VersionID                 string `json:"version_id,omitempty"`
}

type voiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
	Style           *float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool    `json:"use_speaker_boost,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
}

// textToSpeech buffers the entire audio body into memory and returns it
// alongside the response headers (used by callers to surface mime type
// and request id).
func (a *api) textToSpeech(ctx context.Context, voiceID, outputFormat string, body *ttsRequest) ([]byte, http.Header, error) {
	resp, err := a.buildAudioRequest(ctx, outputFormat, body).Post("/text-to-speech/" + voiceID)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.Body(), resp.Header(), nil
}

// textToSpeechStream opts out of resty's response parsing so callers can
// stream audio chunks directly off the wire. The returned ReadCloser
// MUST be closed by the caller.
func (a *api) textToSpeechStream(ctx context.Context, voiceID, outputFormat string, body *ttsRequest) (io.ReadCloser, http.Header, error) {
	req := a.buildAudioRequest(ctx, outputFormat, body).SetDoNotParseResponse(true)
	resp, err := req.Post("/text-to-speech/" + voiceID + "/stream")
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		// Drain + close to surface the error body to the caller.
		raw := resp.RawBody()
		errBody, _ := io.ReadAll(raw)
		_ = raw.Close()
		return nil, nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), string(errBody))
	}
	return resp.RawBody(), resp.Header(), nil
}

func (a *api) buildAudioRequest(ctx context.Context, outputFormat string, body *ttsRequest) *resty.Request {
	req := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body)
	if outputFormat != "" {
		req = req.SetQueryParam("output_format", outputFormat)
	}
	if body.EnableLogging != nil {
		req.SetQueryParam("enable_logging", strconv.FormatBool(*body.EnableLogging))
	}
	if body.OptimizeStreamingLatency != nil {
		req.SetQueryParam("optimize_streaming_latency", strconv.Itoa(*body.OptimizeStreamingLatency))
	}
	return req
}

// TranscriptionRequest mirrors POST /v1/speech-to-text (multipart/form-data).
// Audio is uploaded as the "file" form field; other parameters are form fields.
// It intentionally models only synchronous, single-result options representable
// by Core's transcription protocol.
type transcriptionRequest struct {
	ModelID               string
	LanguageCode          string
	Diarize               *bool
	NumSpeakers           *int
	TagAudioEvents        *bool
	TimestampsGranularity string
	DiarizationThreshold  *float64
	FileFormat            string
	Temperature           *float64
	Seed                  *int
	NoVerbatim            *bool
	UseSpeakerLibrary     *bool
	DetectSpeakerRoles    *bool
	Keyterms              []string
}

// TranscriptionResponse models /v1/speech-to-text JSON output.
type transcriptionResponse struct {
	LanguageCode        string                `json:"language_code"`
	LanguageProbability float64               `json:"language_probability"`
	Text                string                `json:"text"`
	Words               []transcriptionWord   `json:"words"`
	Entities            []transcriptionEntity `json:"entities,omitempty"`
}

type transcriptionWord struct {
	Text         string  `json:"text"`
	Type         string  `json:"type"`
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	SpeakerID    string  `json:"speaker_id,omitempty"`
	ChannelIndex *int    `json:"channel_index,omitempty"`
}

type transcriptionEntity struct {
	Text       string `json:"text"`
	EntityType string `json:"entity_type"`
	StartChar  int    `json:"start_char"`
	EndChar    int    `json:"end_char"`
}

func (a *api) transcription(ctx context.Context, audio []byte, mimeType string, req *transcriptionRequest) (*transcriptionResponse, error) {
	if len(audio) == 0 {
		return nil, errors.New("elevenlabs: transcription audio must not be empty")
	}

	form := map[string]string{}
	if req != nil {
		if req.ModelID != "" {
			form["model_id"] = req.ModelID
		}
		if req.LanguageCode != "" {
			form["language_code"] = req.LanguageCode
		}
		if req.Diarize != nil {
			form["diarize"] = strconv.FormatBool(*req.Diarize)
		}
		if req.NumSpeakers != nil {
			form["num_speakers"] = strconv.Itoa(*req.NumSpeakers)
		}
		if req.TagAudioEvents != nil {
			form["tag_audio_events"] = strconv.FormatBool(*req.TagAudioEvents)
		}
		if req.TimestampsGranularity != "" {
			form["timestamps_granularity"] = req.TimestampsGranularity
		}
		if req.DiarizationThreshold != nil {
			form["diarization_threshold"] = strconv.FormatFloat(*req.DiarizationThreshold, 'f', -1, 64)
		}
		if req.FileFormat != "" {
			form["file_format"] = req.FileFormat
		}
		if req.Temperature != nil {
			form["temperature"] = strconv.FormatFloat(*req.Temperature, 'f', -1, 64)
		}
		if req.Seed != nil {
			form["seed"] = strconv.Itoa(*req.Seed)
		}
		if req.NoVerbatim != nil {
			form["no_verbatim"] = strconv.FormatBool(*req.NoVerbatim)
		}
		if req.UseSpeakerLibrary != nil {
			form["use_speaker_library"] = strconv.FormatBool(*req.UseSpeakerLibrary)
		}
		if req.DetectSpeakerRoles != nil {
			form["detect_speaker_roles"] = strconv.FormatBool(*req.DetectSpeakerRoles)
		}
		if len(req.Keyterms) > 0 {
			encoded, err := json.Marshal(req.Keyterms)
			if err != nil {
				return nil, fmt.Errorf("elevenlabs: encode transcription keyterms: %w", err)
			}
			form["keyterms"] = string(encoded)
		}
	}

	var out transcriptionResponse
	r := a.http.R().
		SetContext(ctx).
		SetMultipartField("file", "audio", cmp.Or(mimeType, "application/octet-stream"), bytes.NewReader(audio)).
		SetMultipartFormData(form).
		SetResult(&out)
	resp, err := r.Post("/speech-to-text")
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
