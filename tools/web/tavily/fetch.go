package tavily

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchRequest struct {
	URLs         []string `json:"urls"`
	ExtractDepth string   `json:"extract_depth,omitempty"`
	Format       string   `json:"format,omitempty"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("tavily: fetch request must not be nil")
	}
	if len(f.URLs) == 0 {
		return errors.New("tavily: fetch URLs must not be empty")
	}
	return nil
}

type fetchResult struct {
	RawContent string `json:"raw_content"`
}

type failedFetchResult struct {
	Error string `json:"error"`
}

type fetchResponse struct {
	Results       []*fetchResult       `json:"results"`
	FailedResults []*failedFetchResult `json:"failed_results"`
}

func (c *Client) fetch(ctx context.Context, request *fetchRequest) (*fetchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	response, err := c.http.R().SetContext(ctx).SetBody(request).SetResult(&raw).Post("/extract")
	if err != nil {
		return nil, fmt.Errorf("tavily: execute fetch request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("tavily: fetch request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, request *web.FetchRequest) (*web.FetchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("tavily: prepare fetch request: %w", err)
	}
	request = prepared
	format := request.Format
	if format == web.FormatHTML {
		format = web.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &fetchRequest{
		URLs:         []string{request.URL},
		ExtractDepth: "basic",
		Format:       string(format),
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		if len(raw.FailedResults) > 0 {
			return nil, fmt.Errorf("tavily: fetch response reported failure: %s", raw.FailedResults[0].Error)
		}
		return nil, errors.New("tavily: fetch response contains no result")
	}
	return &web.FetchResponse{Content: raw.Results[0].RawContent, Format: format}, nil
}
