package exa

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/tools/web"
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
		return errors.New("exa: fetch request must not be nil")
	}
	if len(f.URLs) == 0 {
		return errors.New("exa: fetch URLs must not be empty")
	}
	return nil
}

type fetchResult struct {
	Text string `json:"text,omitempty"`
}

type fetchResponse struct {
	Results []*fetchResult `json:"results"`
}

func (c *Client) fetch(ctx context.Context, request *fetchRequest) (*fetchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	response, err := c.http.R().SetContext(ctx).SetBody(request).SetResult(&raw).Post("/contents")
	if err != nil {
		return nil, fmt.Errorf("exa: execute fetch request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("exa: fetch request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, request *web.FetchRequest) (*web.FetchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("exa: prepare fetch request: %w", err)
	}
	request = prepared
	format := request.Format
	raw, err := c.fetch(ctx, &fetchRequest{
		URLs: []string{request.URL},
		Text: fetchTextOptions{IncludeHTMLTags: format == web.FormatHTML},
	})
	if err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		return nil, errors.New("exa: fetch response contains no result")
	}
	return &web.FetchResponse{Content: raw.Results[0].Text, Format: format}, nil
}
