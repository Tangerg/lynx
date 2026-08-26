package firecrawl

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/tools/web"
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
		return errors.New("firecrawl: Request must not be nil")
	}
	if f.URL == "" {
		return errors.New("firecrawl: URL must not be empty")
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

func (c *Client) fetch(ctx context.Context, req *fetchRequest) (*fetchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	var raw fetchResponse
	resp, err := c.http.R().SetContext(ctx).SetBody(req).SetResult(&raw).Post("/scrape")
	if err != nil {
		return nil, fmt.Errorf("firecrawl: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("firecrawl: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	if !raw.Success {
		return nil, fmt.Errorf("firecrawl: scrape failed: %s", resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *web.FetchRequest) (*web.FetchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("firecrawl: %w", err)
	}
	format := req.Format.Resolve()
	if format == web.FormatText {
		format = web.FormatMarkdown
	}
	raw, err := c.fetch(ctx, &fetchRequest{
		URL:             req.URL,
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
