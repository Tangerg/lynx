package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type interactionErrorEnvelope struct {
	Error struct {
		Code    int               `json:"code"`
		Message string            `json:"message"`
		Status  string            `json:"status"`
		Details []json.RawMessage `json:"details,omitempty"`
	} `json:"error"`
}

func (a *api) createImageInteraction(ctx context.Context, req *imageInteractionRequest) (*imageInteractionResponse, error) {
	if a == nil || a.interactionsHTTP == nil {
		return nil, errors.New("google: image: nil API")
	}
	if req == nil {
		return nil, errors.New("google: image: request must not be nil")
	}

	var out imageInteractionResponse
	var apiErr interactionErrorEnvelope
	resp, err := a.interactionsHTTP.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&out).
		SetError(&apiErr).
		Post("/v1beta/interactions")
	if err != nil {
		return nil, fmt.Errorf("google: image: create interaction: %w", err)
	}
	if !resp.IsSuccess() {
		if apiErr.Error.Message != "" {
			return nil, fmt.Errorf("google: image: http %d (%s): %s", resp.StatusCode(), apiErr.Error.Status, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("google: image: http %d: %s", resp.StatusCode(), resp.String())
	}
	out.Raw = slices.Clone(resp.Body())
	return &out, nil
}
