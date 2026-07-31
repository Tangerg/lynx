package azureopenai

import (
	"errors"
	"net/url"
	"strings"

	"github.com/openai/openai-go/v3/option"
)

func buildRequestOptions(baseURL string, extra []option.RequestOption) ([]option.RequestOption, error) {
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	options := make([]option.RequestOption, 0, len(extra)+1)
	options = append(options, option.WithBaseURL(normalized))
	options = append(options, extra...)
	return options, nil
}

func normalizeBaseURL(baseURL string) (string, error) {
	if baseURL == "" {
		return "", errors.New("azureopenai: BaseURL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.New("azureopenai: BaseURL must be a valid absolute URL")
	}
	if parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("azureopenai: BaseURL must be an absolute URL without credentials, query parameters, or fragments")
	}
	if !strings.HasSuffix(parsed.Path, "/openai/v1") && !strings.HasSuffix(parsed.Path, "/openai/v1/") {
		return "", errors.New("azureopenai: BaseURL must end with /openai/v1/")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/"
	return parsed.String(), nil
}
