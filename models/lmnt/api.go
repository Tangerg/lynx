package lmnt

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-resty/resty/v2"
)

type APIConfig struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

func (c APIConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("lmnt: APIKey is required")
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
	client := resty.New()
	if cfg.HTTPClient != nil {
		client = resty.NewWithClient(cfg.HTTPClient)
	}
	client.SetBaseURL(cmp.Or(cfg.BaseURL, DefaultBaseURL)).
		SetHeader("X-API-Key", cfg.APIKey).
		SetHeader("lmnt-version", CurrentAPIVersion).
		SetHeader("Content-Type", "application/json")
	return &API{http: client}, nil
}

// SynthesizeRequest mirrors the current POST /ai/speech/bytes contract.
type SynthesizeRequest struct {
	Text        string   `json:"text"`
	Voice       string   `json:"voice"`
	Debug       *bool    `json:"debug,omitempty"`
	Format      string   `json:"format,omitempty"`
	Language    string   `json:"language,omitempty"`
	Model       string   `json:"model,omitempty"`
	SampleRate  int      `json:"sample_rate,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	Seed        *int64   `json:"seed,omitempty"`
}

// Synthesize returns the official binary response and response headers.
func (a *API) Synthesize(ctx context.Context, req *SynthesizeRequest) ([]byte, http.Header, error) {
	body, headers, err := a.SynthesizeStream(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	defer body.Close()
	audio, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("lmnt: read speech response: %w", err)
	}
	if len(audio) == 0 {
		return nil, nil, errors.New("lmnt: speech response is empty")
	}
	return audio, headers, nil
}

// SynthesizeStream exposes the official binary response as it arrives.
func (a *API) SynthesizeStream(ctx context.Context, req *SynthesizeRequest) (io.ReadCloser, http.Header, error) {
	if req == nil {
		return nil, nil, errors.New("lmnt: request must not be nil")
	}
	resp, err := a.http.R().
		SetContext(ctx).
		SetBody(req).
		SetDoNotParseResponse(true).
		Post("/ai/speech/bytes")
	if err != nil {
		return nil, nil, fmt.Errorf("lmnt: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		defer resp.RawBody().Close()
		body, readErr := io.ReadAll(resp.RawBody())
		if readErr != nil {
			return nil, nil, fmt.Errorf("lmnt: http %d; read error response: %w", resp.StatusCode(), readErr)
		}
		return nil, nil, fmt.Errorf("lmnt: http %d: %s", resp.StatusCode(), string(body))
	}
	if resp.RawBody() == nil {
		return nil, nil, errors.New("lmnt: speech response has no body")
	}
	return resp.RawBody(), resp.Header(), nil
}
