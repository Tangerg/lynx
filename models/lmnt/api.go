package lmnt

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

type ApiConfig struct {
	ApiKey     model.ApiKey
	BaseURL    string
	HTTPClient *http.Client
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("lmnt: config must not be nil")
	}
	if c.ApiKey == nil {
		return errors.New("lmnt: ApiKey is required")
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
		SetHeader("X-API-Key", cfg.ApiKey.Get()).
		SetHeader("Content-Type", "application/json")
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}
	return &Api{http: client}, nil
}

// SynthesizeRequest mirrors POST /ai/speech/bytes. LMNT returns JSON
// with base64-encoded audio under the "audio" key, plus a seed and
// optional durations.
type SynthesizeRequest struct {
	Text            string   `json:"text"`
	Voice           string   `json:"voice"`
	Model           string   `json:"model,omitempty"`
	Format          string   `json:"format,omitempty"`
	SampleRate      int      `json:"sample_rate,omitempty"`
	Speed           *float64 `json:"speed,omitempty"`
	Language        string   `json:"language,omitempty"`
	Seed            *int64   `json:"seed,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	ReturnDurations *bool    `json:"return_durations,omitempty"`
}

// SynthesizeResponse is the JSON envelope. Audio is base64-encoded
// bytes — callers decode with [SynthesizeResponse.Decode] to get the
// raw audio buffer.
type SynthesizeResponse struct {
	Audio     string `json:"audio"`
	Seed      int64  `json:"seed"`
	Durations []any  `json:"durations,omitzero"`
}

func (s *SynthesizeResponse) Decode() ([]byte, error) {
	return base64.StdEncoding.DecodeString(s.Audio)
}

func (a *Api) Synthesize(ctx context.Context, req *SynthesizeRequest) (*SynthesizeResponse, error) {
	if req == nil {
		return nil, errors.New("lmnt: request must not be nil")
	}
	var out SynthesizeResponse
	resp, err := a.http.R().SetContext(ctx).SetBody(req).SetResult(&out).Post("/ai/speech/bytes")
	if err != nil {
		return nil, fmt.Errorf("lmnt: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("lmnt: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}
