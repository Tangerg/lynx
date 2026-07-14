package chatclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

var (
	// ErrNilClient reports a structured call without a Client.
	ErrNilClient = errors.New("chatclient: nil client")
	// ErrNoUserMessage reports that output instructions have nowhere to be
	// attached in a chat request.
	ErrNoUserMessage = errors.New("chatclient: no user message for output instructions")
)

// CallStructured appends output instructions to the last user message, calls
// client, and decodes the first response choice into T. It preserves the raw
// response on decode failure so callers can inspect provider metadata or
// implement a repair policy.
//
// This is a package function because Go does not support type-parameterized
// methods. Client therefore keeps its direct Call/Stream method surface.
func CallStructured[T any](
	ctx context.Context,
	client *Client,
	request *chat.Request,
	output Output[T],
) (T, *chat.Response, error) {
	var zero T
	if client == nil {
		return zero, nil, ErrNilClient
	}
	if err := output.Validate(); err != nil {
		return zero, nil, err
	}

	prepared, err := prepareRequest(request, chat.Options{})
	if err != nil {
		return zero, nil, err
	}
	if output.Instructions != "" {
		if err := appendOutputInstructions(prepared, output.Instructions); err != nil {
			return zero, nil, err
		}
	}

	response, err := client.Call(ctx, prepared)
	if err != nil {
		return zero, response, err
	}
	decoded, err := output.Decode(response.Text())
	if err != nil {
		return zero, response, fmt.Errorf("chatclient: structured output: %w", err)
	}
	return decoded, response, nil
}

func appendOutputInstructions(request *chat.Request, instructions string) error {
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role != chat.RoleUser {
			continue
		}
		text := instructions
		if request.Messages[i].Text() != "" {
			text = "\n\n" + instructions
		}
		request.Messages[i].Parts = append(request.Messages[i].Parts, chat.NewTextPart(text))
		if err := request.Messages[i].Validate(); err != nil {
			return fmt.Errorf("%w: append instructions: %w", ErrInvalidOutput, err)
		}
		return nil
	}
	return ErrNoUserMessage
}
