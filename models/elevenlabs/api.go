package elevenlabs

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/model"
)

type ApiConfig struct {
	ApiKey     model.ApiKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("elevenlabs: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("elevenlabs: ApiKey is required")
	}
	return nil
}

type Api struct {
	http *resty.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetHeader("xi-api-key", cfg.ApiKey.Get()).
		SetHeader("Accept", "audio/*")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}

	return &Api{http: client}, nil
}

type TTSRequest struct {
	Text                           string         `json:"text"`
	ModelID                        string         `json:"model_id,omitempty"`
	LanguageCode                   string         `json:"language_code,omitempty"`
	VoiceSettings                  *VoiceSettings `json:"voice_settings,omitempty"`
	Seed                           *int64         `json:"seed,omitempty"`
	PreviousText                   string         `json:"previous_text,omitempty"`
	NextText                       string         `json:"next_text,omitempty"`
	PreviousRequestIDs             []string       `json:"previous_request_ids,omitzero"`
	NextRequestIDs                 []string       `json:"next_request_ids,omitzero"`
	ApplyTextNormalization         string         `json:"apply_text_normalization,omitempty"`
	ApplyLanguageTextNormalization *bool          `json:"apply_language_text_normalization,omitempty"`
}

type VoiceSettings struct {
	Stability       *float64 `json:"stability,omitempty"`
	SimilarityBoost *float64 `json:"similarity_boost,omitempty"`
	Style           *float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool    `json:"use_speaker_boost,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
}

// TextToSpeech buffers the entire audio body into memory and returns it
// alongside the response headers (used by callers to surface mime type
// and request id).
func (a *Api) TextToSpeech(ctx context.Context, voiceID, outputFormat string, body *TTSRequest) ([]byte, http.Header, error) {
	resp, err := a.buildAudioRequest(ctx, outputFormat, body).Post("/text-to-speech/" + voiceID)
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.Body(), resp.Header(), nil
}

// TextToSpeechStream opts out of resty's response parsing so callers can
// stream audio chunks directly off the wire. The returned ReadCloser
// MUST be closed by the caller.
func (a *Api) TextToSpeechStream(ctx context.Context, voiceID, outputFormat string, body *TTSRequest) (io.ReadCloser, http.Header, error) {
	req := a.buildAudioRequest(ctx, outputFormat, body).SetDoNotParseResponse(true)
	resp, err := req.Post("/text-to-speech/" + voiceID + "/stream")
	if err != nil {
		return nil, nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		// Drain + close so we can surface the error body to the caller.
		raw := resp.RawBody()
		errBody, _ := io.ReadAll(raw)
		_ = raw.Close()
		return nil, nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), string(errBody))
	}
	return resp.RawBody(), resp.Header(), nil
}

func (a *Api) buildAudioRequest(ctx context.Context, outputFormat string, body *TTSRequest) *resty.Request {
	req := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body)
	if outputFormat != "" {
		req = req.SetQueryParam("output_format", outputFormat)
	}
	return req
}

// TranscriptionRequest mirrors POST /v1/speech-to-text (multipart/form-data).
// Audio is uploaded as the "file" form field; other parameters ride as
// form fields too. The "scribe_v1" model identifier is what ElevenLabs
// expects in 2025.
type TranscriptionRequest struct {
	ModelID               string
	LanguageCode          string
	Diarize               *bool
	NumSpeakers           *int
	TagAudioEvents        *bool
	TimestampsGranularity string
	Tags                  []string
}

// TranscriptionResponse models /v1/speech-to-text JSON output.
type TranscriptionResponse struct {
	LanguageCode        string  `json:"language_code"`
	LanguageProbability float64 `json:"language_probability"`
	Text                string  `json:"text"`
	Words               []struct {
		Text      string  `json:"text"`
		Type      string  `json:"type"`
		Start     float64 `json:"start"`
		End       float64 `json:"end"`
		SpeakerID string  `json:"speaker_id,omitempty"`
	} `json:"words"`
}

func (a *Api) Transcription(ctx context.Context, audio []byte, mimeType string, req *TranscriptionRequest) (*TranscriptionResponse, error) {
	if len(audio) == 0 {
		return nil, errors.New("elevenlabs: request must not be nil")
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
			form["diarize"] = boolStr(*req.Diarize)
		}
		if req.NumSpeakers != nil {
			form["num_speakers"] = intStr(*req.NumSpeakers)
		}
		if req.TagAudioEvents != nil {
			form["tag_audio_events"] = boolStr(*req.TagAudioEvents)
		}
		if req.TimestampsGranularity != "" {
			form["timestamps_granularity"] = req.TimestampsGranularity
		}
	}

	var out TranscriptionResponse
	r := a.http.R().
		SetContext(ctx).
		SetFileReader("file", "audio", bytes.NewReader(audio)).
		SetMultipartFormData(form).
		SetResult(&out)
	if mimeType != "" {
		r.SetHeader("Content-Type", mimeType)
	}
	resp, err := r.Post("/speech-to-text")
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("elevenlabs: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func intStr(v int) string {
	return strconv.Itoa(v)
}
