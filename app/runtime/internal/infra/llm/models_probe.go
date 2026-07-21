package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	probeTimeout = 4 * time.Second
	probeMaxBody = 1 << 20 // 1 MiB — a model list is tiny; cap a hostile/huge body.
)

// remoteModelList is the OpenAI GET /v1/models response shape, which Ollama /
// LM Studio / vLLM / OpenRouter and Anthropic's list endpoint all emit. Only the
// ids matter here; capability/pricing are enriched from the static catalog by
// the caller when the id is known.
type remoteModelList struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListRemoteModels probes an OpenAI-compatible provider's model endpoint
// (GET {baseURL}/models) and returns the advertised model ids, sorted and
// de-duplicated. It backs live model discovery for local / bring-your-own-
// endpoint providers whose model set is user-defined rather than in the static
// catalog (Ollama, the compat passthroughs). apiKey rides as a bearer token when
// non-empty (a local daemon needs none). The call is bounded (timeout + response
// cap); a non-200 or unparseable body is returned as an error the caller treats
// as "no discovery" and falls back to the static catalog.
func ListRemoteModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: model probe %s: status %d", endpoint, resp.StatusCode)
	}

	var list remoteModelList
	if err := json.NewDecoder(io.LimitReader(resp.Body, probeMaxBody)).Decode(&list); err != nil {
		return nil, fmt.Errorf("llm: model probe %s: %w", endpoint, err)
	}

	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
