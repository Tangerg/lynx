package ollama

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"

	ollamaapi "github.com/ollama/ollama/api"
)

// ApiConfig configures the Ollama native client. Unlike cloud
// providers, Ollama runs locally so the typical config is just the
// BaseURL of the daemon (default: http://127.0.0.1:11434). There is no
// API key.
type ApiConfig struct {
	// BaseURL points at the Ollama daemon. Empty falls back to
	// [DefaultBaseURL]. Pass an env value like
	// "https://ollama.internal:11434" for remote setups.
	BaseURL string

	// HTTPClient lets callers thread their own client through. nil
	// falls back to [http.DefaultClient].
	HTTPClient *http.Client
}

func (c *ApiConfig) validate() error {
	if c == nil {
		return errors.New("ollama: config must not be nil")
	}
	return nil
}

// Api wraps Ollama's native client. We use the official SDK's typed
// surface for chat/embed (the SDK is hosted at github.com/ollama/ollama/api
// in the same repo as the daemon).
type Api struct {
	client *ollamaapi.Client
}

func NewApi(cfg *ApiConfig) (*Api, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	u, err := url.Parse(cmp.Or(cfg.BaseURL, DefaultBaseURL))
	if err != nil {
		return nil, errors.New("ollama: BaseURL must be a valid URL")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Api{client: ollamaapi.NewClient(u, httpClient)}, nil
}

// Chat wraps client.Chat. The Ollama SDK uses a streaming callback for
// both sync and stream paths — Stream=false on the request still goes
// through the callback but fires exactly once with the complete reply.
func (a *Api) Chat(ctx context.Context, req *ollamaapi.ChatRequest, fn ollamaapi.ChatResponseFunc) error {
	if req == nil {
		return errors.New("ollama: request must not be nil")
	}
	return a.client.Chat(ctx, req, fn)
}

// Embed wraps client.Embed.
func (a *Api) Embed(ctx context.Context, req *ollamaapi.EmbedRequest) (*ollamaapi.EmbedResponse, error) {
	if req == nil {
		return nil, errors.New("ollama: request must not be nil")
	}
	return a.client.Embed(ctx, req)
}
