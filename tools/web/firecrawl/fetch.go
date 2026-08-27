package firecrawl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchFormat struct {
	Type string `json:"type"`
}

type fetchRequest struct {
	URL             string        `json:"url"`
	Formats         []fetchFormat `json:"formats"`
	OnlyMainContent bool          `json:"onlyMainContent"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("firecrawl: fetch request must not be nil")
	}
	if f.URL == "" {
		return errors.New("firecrawl: fetch URL must not be empty")
	}
	return nil
}

type fetchResponseData struct {
	Markdown string  `json:"markdown,omitempty"`
	HTML     *string `json:"html,omitempty"`
}

type fetchResponse struct {
	Success bool              `json:"success"`
	Data    fetchResponseData `json:"data"`
}

func (c *Client) fetch(ctx context.Context, request *fetchRequest) (*fetchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	response, err := c.http.R().SetContext(ctx).SetBody(request).SetResult(&raw).Post("/scrape")
	if err != nil {
		return nil, fmt.Errorf("firecrawl: execute fetch request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("firecrawl: fetch request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	if !raw.Success {
		return nil, fmt.Errorf("firecrawl: fetch response reported failure: %s", response.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, request *web.FetchRequest) (*web.FetchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("firecrawl: prepare fetch request: %w", err)
	}
	request = prepared
	format := request.Format
	if format == web.FormatText {
		format = web.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &fetchRequest{
		URL:             request.URL,
		Formats:         []fetchFormat{{Type: string(format)}},
		OnlyMainContent: true,
	})
	if err != nil {
		return nil, err
	}
	content := raw.Data.Markdown
	if format == web.FormatHTML && raw.Data.HTML != nil {
		content = *raw.Data.HTML
	}
	return &web.FetchResponse{Content: content, Format: format}, nil
}
