package hume

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"

	"github.com/Tangerg/lynx/core/model"
)

type APIConfig struct {
	APIKey     model.APIKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("hume: APIKey is required")
	}
	return nil
}

type API struct {
	http *resty.Client
}

func NewAPI(cfg APIConfig) (*API, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	client := resty.New().
		SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetHeader("X-Hume-API-Key", cfg.APIKey.Get()).
		SetHeader("Content-Type", "application/json")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &API{http: client}, nil
}

// Voice references a named Octave voice. Provider is "HUME_AI" /
// "CUSTOM_VOICE" depending on where the voice is stored.
type Voice struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider,omitempty"`
}

// Utterance is the per-segment input to TTS. "Description" is the
// emotion / style cue Octave is famous for (e.g. "calm, professional");
// it can replace Voice for fully-prompt-driven generation.
type Utterance struct {
	Text            string   `json:"text"`
	Description     string   `json:"description,omitempty"`
	Voice           *Voice   `json:"voice,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
	TrailingSilence *float64 `json:"trailing_silence,omitempty"`
}

// TTSRequest mirrors POST /tts. Format is "mp3" / "wav" / "pcm";
// SplitUtterances controls whether the response includes per-utterance
// timing.
type TTSRequest struct {
	Utterances      []Utterance    `json:"utterances"`
	Context         map[string]any `json:"context,omitzero"`
	Format          map[string]any `json:"format,omitzero"`
	NumGenerations  int            `json:"num_generations,omitempty"`
	SplitUtterances *bool          `json:"split_utterances,omitempty"`
	StripHeaders    *bool          `json:"strip_headers,omitempty"`
}

// TTSResponse is the JSON envelope. Generations[0].Audio is the
// base64-encoded audio bytes.
type TTSResponse struct {
	Generations []struct {
		ID       string `json:"generation_id"`
		Audio    string `json:"audio"`
		Encoding struct {
			Format     string `json:"format"`
			SampleRate int    `json:"sample_rate"`
		} `json:"encoding"`
		Duration float64 `json:"duration"`
	} `json:"generations"`
	RequestID string `json:"request_id"`
}

// DecodeAudio returns the raw audio bytes from the first generation.
func (r *TTSResponse) DecodeAudio() ([]byte, error) {
	if len(r.Generations) == 0 {
		return nil, errors.New("hume: request must not be nil")
	}
	return base64.StdEncoding.DecodeString(r.Generations[0].Audio)
}

func (a *API) TTS(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	if req == nil {
		return nil, errors.New("hume: request must not be nil")
	}
	var out TTSResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post("/tts")
	if err != nil {
		return nil, fmt.Errorf("hume: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("hume: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
