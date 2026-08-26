package exa

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchTextOptions struct {
	IncludeHTMLTags bool `json:"includeHtmlTags,omitempty"`
}

type fetchRequest struct {
	URLs []string         `json:"urls,omitempty"`
	Text fetchTextOptions `json:"text,omitempty"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("exa: Request must not be nil")
	}
	if len(f.URLs) == 0 {
		return errors.New("exa: URLs must be non-empty")
	}
	return nil
}

type fetchResult struct {
	Text string `json:"text,omitempty"`
}

type fetchResponse struct {
	Results []*fetchResult `json:"results"`
}

func (c *Client) fetch(ctx context.Context, req *fetchRequest) (*fetchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/contents")
	if err != nil {
		return nil, fmt.Errorf("exa: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("exa: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *web.FetchRequest) (*web.FetchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("exa: %w", err)
	}
	format := req.Format.Resolve()
	raw, err := c.fetch(ctx, &fetchRequest{
		URLs: []string{req.URL},
		Text: fetchTextOptions{IncludeHTMLTags: format == web.FormatHTML},
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		return nil, fmt.Errorf("exa: empty result for %s", req.URL)
	}
	return &web.FetchResponse{Content: raw.Results[0].Text, Format: format}, nil
}
