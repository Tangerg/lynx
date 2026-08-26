package jina

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchRequest struct {
	URL          string `json:"url"`
	ReturnFormat string `json:"-"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("jina: Request must not be nil")
	}
	if f.URL == "" {
		return errors.New("jina: URL must not be empty")
	}
	return nil
}

type fetchResponseData struct {
	Content string `json:"content"`
}

type fetchResponse struct {
	Data fetchResponseData `json:"data"`
}

func (c *Client) fetch(ctx context.Context, req *fetchRequest) (*fetchResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	httpRequest := c.fetchHTTP.R().SetContext(ctx).
		SetBody(req).
		SetHeader("X-Retain-Images", "none")
	if req.ReturnFormat != "" {
		httpRequest.SetHeader("X-Return-Format", req.ReturnFormat)
	}

	var raw fetchResponse
	resp, err := httpRequest.SetResult(&raw).Post("/")
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("jina: API error (status %d): %s", resp.StatusCode(), resp.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, req *web.FetchRequest) (*web.FetchResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("jina: %w", err)
	}
	format := req.Format.Resolve()
	raw, err := c.fetch(ctx, &fetchRequest{URL: req.URL, ReturnFormat: string(format)})
	if err != nil {
		return nil, err
	}
	return &web.FetchResponse{Content: raw.Data.Content, Format: format}, nil
}
