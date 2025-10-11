package embedding

import (
	"context"
	"errors"
)

type Client struct {
	model Model
}

func NewClient(model Model) (*Client, error) {
	if model == nil {
		return nil, errors.New("model is required")
	}
	return &Client{
		model: model,
	}, nil
}

func (c *Client) Embed(ctx context.Context, req *Request) (*Response, error) {
	return c.model.Call(ctx, req)
}

func (c *Client) EmbedText(ctx context.Context, text string) (*Response, error) {
	return c.EmbedTexts(ctx, []string{text})
}

func (c *Client) EmbedTexts(ctx context.Context, texts []string) (*Response, error) {
	req, err := NewRequest(texts)
	if err != nil {
		return nil, err
	}

	return c.Embed(ctx, req)
}
