package tavily

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchRequest struct {
	URLs         []string `json:"urls"`
	ExtractDepth string   `json:"extract_depth,omitempty"`
	Format       string   `json:"format,omitempty"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("tavily: Request must not be nil")
	}
	if len(f.URLs) == 0 {
		return errors.New("tavily: URLs must not be empty")
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

func (c *Client) fetch(ctx context.Context, req *fetchRequest) (*fetchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/extract")
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("tavily: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *web.FetchRequest) (*web.FetchResponse, error) {
	prepared, err := req.Prepare()
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	req = prepared
	format := req.Format
	if format == web.FormatHTML {
		format = web.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &fetchRequest{
		URLs:         []string{req.URL},
		ExtractDepth: "basic",
		Format:       string(format),
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		if len(raw.FailedResults) > 0 {
			return nil, fmt.Errorf("tavily: extract failed: %s", raw.FailedResults[0].Error)
		}
		return nil, fmt.Errorf("tavily: empty result for %s", req.URL)
	}
	return &web.FetchResponse{Content: raw.Results[0].RawContent, Format: format}, nil
}
