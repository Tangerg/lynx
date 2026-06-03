package deepgram

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

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
		return errors.New("deepgram: APIKey is required")
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
		SetHeader("Authorization", "Token "+cfg.APIKey.Get())
	if cfg.HTTPClient != nil {
		client.SetTransport(cfg.HTTPClient.Transport)
	}

	return &API{http: client}, nil
}

// ListenParams holds the query-string knobs Deepgram /listen accepts.
// Extra is appended as-is for params not surfaced as typed fields.
type ListenParams struct {
	Model        string
	Language     string
	Tier         string
	Version      string
	Punctuate    *bool
	SmartFormat  *bool
	Diarize      *bool
	Numerals     *bool
	Paragraphs   *bool
	Utterances   *bool
	DetectTopics *bool
	Summarize    string
	Redact       []string
	Extra        url.Values
}

type ListenResponse struct {
	RequestID string `json:"request_id"`
	Metadata  struct {
		TransactionKey string   `json:"transaction_key"`
		RequestID      string   `json:"request_id"`
		Channels       int      `json:"channels"`
		Duration       float64  `json:"duration"`
		Models         []string `json:"models"`
		Metadata       map[string]struct {
			Name string `json:"name"`
			Tier string `json:"tier"`
		} `json:"model_info"`
	} `json:"metadata"`
	Results struct {
		Channels []struct {
			Alternatives []struct {
				Transcript string  `json:"transcript"`
				Confidence float64 `json:"confidence"`
				Words      []struct {
					Word    string  `json:"word"`
					Start   float64 `json:"start"`
					End     float64 `json:"end"`
					Speaker int     `json:"speaker,omitempty"`
				} `json:"words"`
			} `json:"alternatives"`
		} `json:"channels"`
		Utterances []struct {
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Speaker    int     `json:"speaker"`
			Transcript string  `json:"transcript"`
		} `json:"utterances,omitempty"`
	} `json:"results"`
}

func (a *API) Listen(ctx context.Context, audio []byte, contentType string, params *ListenParams) (*ListenResponse, error) {
	if len(audio) == 0 {
		return nil, errors.New("deepgram: request must not be nil")
	}

	var out ListenResponse
	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", cmp.Or(contentType, "application/octet-stream")).
		SetQueryParamsFromValues(buildListenQuery(params)).
		SetBody(audio).
		SetResult(&out).
		Post("/listen")
	if err != nil {
		return nil, fmt.Errorf("deepgram: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("deepgram: http %d: %s", resp.StatusCode(), resp.String())
	}
	return &out, nil
}

// SpeakParams holds the query-string knobs Deepgram /speak accepts.
// See https://developers.deepgram.com/reference/text-to-speech-api.
type SpeakParams struct {
	Model      string // "aura-asteria-en" / "aura-2-thalia-en" etc.
	Encoding   string // "mp3" / "linear16" / "opus" / "flac" / "aac" / "mulaw" / "alaw"
	Container  string // "wav" / "none"
	SampleRate int
	BitRate    int
	Extra      url.Values
}

// Speak posts text to /speak and returns the raw audio bytes plus the
// response headers (request id / content-type live there).
func (a *API) Speak(ctx context.Context, text string, params *SpeakParams) ([]byte, http.Header, error) {
	if text == "" {
		return nil, nil, errors.New("deepgram: request must not be nil")
	}

	resp, err := a.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "audio/*").
		SetQueryParamsFromValues(buildSpeakQuery(params)).
		SetBody(map[string]string{"text": text}).
		Post("/speak")
	if err != nil {
		return nil, nil, fmt.Errorf("deepgram: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, nil, fmt.Errorf("deepgram: http %d: %s", resp.StatusCode(), resp.String())
	}
	return resp.Body(), resp.Header(), nil
}

func buildSpeakQuery(p *SpeakParams) url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	for k, vs := range p.Extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	if p.Model != "" {
		q.Set("model", p.Model)
	}
	if p.Encoding != "" {
		q.Set("encoding", p.Encoding)
	}
	if p.Container != "" {
		q.Set("container", p.Container)
	}
	if p.SampleRate > 0 {
		q.Set("sample_rate", strconv.Itoa(p.SampleRate))
	}
	if p.BitRate > 0 {
		q.Set("bit_rate", strconv.Itoa(p.BitRate))
	}
	return q
}

func buildListenQuery(p *ListenParams) url.Values {
	q := url.Values{}
	if p == nil {
		return q
	}
	for k, vs := range p.Extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	setStr := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	setBool := func(k string, v *bool) {
		if v == nil {
			return
		}
		if *v {
			q.Set(k, "true")
		} else {
			q.Set(k, "false")
		}
	}
	setStr("model", p.Model)
	setStr("language", p.Language)
	setStr("tier", p.Tier)
	setStr("version", p.Version)
	setStr("summarize", p.Summarize)
	setBool("punctuate", p.Punctuate)
	setBool("smart_format", p.SmartFormat)
	setBool("diarize", p.Diarize)
	setBool("numerals", p.Numerals)
	setBool("paragraphs", p.Paragraphs)
	setBool("utterances", p.Utterances)
	setBool("detect_topics", p.DetectTopics)
	for _, r := range p.Redact {
		q.Add("redact", r)
	}
	return q
}
