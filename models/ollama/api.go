package ollama

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	maxNativeStreamFrameBytes = 8 << 20
	maxNativeResponseBytes    = 64 << 20
	maxNativeErrorBytes       = 64 << 10
)

// APIConfig configures the Ollama native client. Unlike cloud
// providers, Ollama runs locally so the typical config is just the
// BaseURL of the daemon (default: http://127.0.0.1:11434). There is no
// API key.
type apiConfig struct {
	// BaseURL points at the Ollama daemon. Empty falls back to
	// [DefaultBaseURL]. Pass an env value like
	// "https://ollama.internal:11434" for remote setups.
	BaseURL string

	// HTTPClient lets callers thread their own client through. nil
	// falls back to [http.DefaultClient].
	HTTPClient *http.Client
}

func (c apiConfig) validate() error {
	return nil
}

// api is the narrow client mechanism for the two native endpoints this adapter
// owns. It deliberately does not import Ollama's daemon module.
type api struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func newAPI(cfg apiConfig) (*api, error) {
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

	return &api{baseURL: u, httpClient: httpClient}, nil
}

// chat decodes Ollama's newline-delimited response for both streaming and
// non-streaming requests. With stream=false the daemon returns one frame.
func (a *api) chat(ctx context.Context, req *nativeChatRequest, fn func(nativeChatResponse) error) error {
	if req == nil {
		return errors.New("ollama: request must not be nil")
	}
	return a.stream(ctx, "/api/chat", req, func(data []byte) error {
		var response nativeChatResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return err
		}
		return fn(response)
	})
}

func (a *api) embed(ctx context.Context, req *nativeEmbedRequest) (*nativeEmbedResponse, error) {
	if req == nil {
		return nil, errors.New("ollama: request must not be nil")
	}
	var response nativeEmbedResponse
	if err := a.call(ctx, "/api/embed", req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (a *api) call(ctx context.Context, path string, requestValue, responseValue any) error {
	response, err := a.request(ctx, path, requestValue, "application/json")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxNativeResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxNativeResponseBytes {
		return fmt.Errorf("ollama: response exceeds %d bytes", maxNativeResponseBytes)
	}
	if err := nativeResponseError(response, body); err != nil {
		return err
	}
	if len(body) == 0 || responseValue == nil {
		return nil
	}
	return json.Unmarshal(body, responseValue)
}

func (a *api) stream(
	ctx context.Context,
	path string,
	requestValue any,
	yield func([]byte) error,
) error {
	response, err := a.request(ctx, path, requestValue, "application/x-ndjson")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxNativeErrorBytes+1))
		if readErr != nil {
			return readErr
		}
		if len(body) > maxNativeErrorBytes {
			body = body[:maxNativeErrorBytes]
		}
		return nativeResponseError(response, body)
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxNativeStreamFrameBytes)
	for scanner.Scan() {
		data := bytes.Clone(scanner.Bytes())
		if err := nativeResponseError(response, data); err != nil {
			return err
		}
		var providerFailure struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(data, &providerFailure); err != nil {
			return err
		}
		if providerFailure.Error != "" {
			return errors.New(providerFailure.Error)
		}
		if err := yield(data); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ollama: read native stream: %w", err)
	}
	return nil
}

func (a *api) request(
	ctx context.Context,
	path string,
	requestValue any,
	accept string,
) (*http.Response, error) {
	body, err := json.Marshal(requestValue)
	if err != nil {
		return nil, err
	}
	endpoint := a.baseURL.JoinPath(path)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "lynx-ollama")
	return a.httpClient.Do(request)
}

type nativeStatusError struct {
	statusCode int
	status     string
	message    string
}

func (err nativeStatusError) Error() string {
	switch {
	case err.status != "" && err.message != "":
		return err.status + ": " + err.message
	case err.status != "":
		return err.status
	case err.statusCode != 0 && err.message != "":
		return fmt.Sprintf("%d: %s", err.statusCode, err.message)
	case err.statusCode != 0:
		return fmt.Sprintf("ollama: HTTP status %d", err.statusCode)
	case err.message != "":
		return err.message
	default:
		return "ollama: request failed"
	}
}

func nativeResponseError(response *http.Response, body []byte) error {
	if response.StatusCode < http.StatusBadRequest {
		return nil
	}
	var providerFailure struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &providerFailure); err != nil || providerFailure.Error == "" {
		providerFailure.Error = string(body)
	}
	return nativeStatusError{
		statusCode: response.StatusCode,
		status:     response.Status,
		message:    providerFailure.Error,
	}
}
