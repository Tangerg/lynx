package interaction

import (
	"bytes"
	"context"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
)

// ModelResponseDelta is one validated provider-neutral streaming increment.
// It is observational and never a source for final Output or restoration.
type ModelResponseDelta struct {
	response chat.Response
}

// ParseModelResponseDelta strictly decodes an Interaction model Delta payload.
func ParseModelResponseDelta(payload json.RawMessage) (ModelResponseDelta, error) {
	var wire modelResponseDeltaWire
	if err := jsonv2.Unmarshal(payload, &wire, jsonv2.RejectUnknownMembers(true)); err != nil {
		return ModelResponseDelta{}, fmt.Errorf("interaction: decode model response Delta: %w", err)
	}
	if err := wire.Response.Validate(); err != nil {
		return ModelResponseDelta{}, fmt.Errorf("interaction: model response Delta: %w", err)
	}
	return ModelResponseDelta{response: *wire.Response.Clone()}, nil
}

// Response returns an independently owned response chunk.
func (m ModelResponseDelta) Response() *chat.Response {
	return m.response.Clone()
}

type modelResponseDeltaWire struct {
	Response chat.Response `json:"response"`
}

func encodeModelResponseDelta(response *chat.Response) (json.RawMessage, error) {
	if response == nil {
		return nil, errors.New("interaction: cannot encode a nil model response Delta")
	}
	payload, err := json.Marshal(modelResponseDeltaWire{
		Response: *response.Clone(),
	})
	if err != nil {
		return nil, fmt.Errorf("interaction: encode model response Delta: %w", err)
	}
	return bytes.Clone(payload), nil
}

func (d *Dispatcher) callModel(
	ctx context.Context,
	request *chat.Request,
	emit agent.DeltaEmitter,
) (*chat.Response, error) {
	if !d.stream {
		return d.client.Call(ctx, request)
	}
	var accumulator chat.ResponseAccumulator
	seen := false
	for delta, err := range d.client.Stream(ctx, request) {
		if err != nil {
			return nil, err
		}
		if delta == nil {
			return nil, errors.New("model stream yielded a nil response Delta")
		}
		if err := accumulator.Add(delta); err != nil {
			return nil, fmt.Errorf("accumulate model stream: %w", err)
		}
		payload, err := encodeModelResponseDelta(delta)
		if err != nil {
			return nil, err
		}
		seen = true
		if emit != nil {
			emit(payload)
		}
	}
	if !seen {
		return nil, errors.New("model stream ended without a response Delta")
	}
	return accumulator.Response(), nil
}
