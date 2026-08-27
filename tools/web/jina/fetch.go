package jina

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/tools/web"
)

var _ web.Fetcher = (*Client)(nil)

type fetchRequest struct {
	URL          string `json:"url"`
	ReturnFormat string `json:"-"`
}

func (f *fetchRequest) validate() error {
	if f == nil {
		return errors.New("jina: fetch request must not be nil")
	}
	if f.URL == "" {
		return errors.New("jina: fetch URL must not be empty")
	}
	return nil
}

type fetchResponseData struct {
	Content string `json:"content"`
}

type fetchResponse struct {
	Data fetchResponseData `json:"data"`
}

func (c *Client) fetch(ctx context.Context, request *fetchRequest) (*fetchResponse, error) {
	if err := request.validate(); err != nil {
		return nil, err
	}
	httpRequest := c.fetchHTTP.R().SetContext(ctx).
		SetBody(request).
		SetHeader("X-Retain-Images", "none")
	if request.ReturnFormat != "" {
		httpRequest.SetHeader("X-Return-Format", request.ReturnFormat)
	}

	var raw fetchResponse
	response, err := httpRequest.SetResult(&raw).Post("/")
	if err != nil {
		return nil, fmt.Errorf("jina: execute fetch request: %w", err)
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("jina: fetch request returned HTTP %d: %s", response.StatusCode(), response.String())
	}
	return &raw, nil
}

func (c *Client) Fetch(ctx context.Context, request *web.FetchRequest) (*web.FetchResponse, error) {
	prepared, err := request.Prepare()
	if err != nil {
		return nil, fmt.Errorf("jina: prepare fetch request: %w", err)
	}
	request = prepared
	format := request.Format
	raw, err := c.fetch(ctx, &fetchRequest{URL: request.URL, ReturnFormat: string(format)})
	if err != nil {
		return nil, err
	}
	return &web.FetchResponse{Content: raw.Data.Content, Format: format}, nil
}
