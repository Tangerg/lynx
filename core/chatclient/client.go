package chatclient

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
)

var (
	ErrNilModel                      = errors.New("chatclient: nil model")
	ErrNilClient                     = errors.New("chatclient: nil client")
	ErrStreamingUnsupported          = errors.New("chatclient: streaming unsupported")
	ErrInputTokenCountingUnsupported = errors.New("chatclient: input token counting unsupported")
)

var errNilStreamSequence = errors.New("chatclient: streamer returned a nil sequence")

type inputTokenCounter interface {
	CountInputTokens(context.Context, *chat.Request) (int64, error)
}

// Client is an immutable, concurrency-safe composition of chat capabilities,
// defaults, and middleware. It does not make an underlying model concurrency
// safe; callers must still follow the model's concurrency contract.
//
// Call and Stream accept ordinary [chat.Request] values directly. Client
// snapshots each request before invoking middleware or a provider, so those
// layers cannot mutate caller-owned protocol values. A configured Streamer
// takes precedence; otherwise New discovers the capability on the model. Stream
// remains lazy and reports unsupported streaming as its single terminal item.
type Client struct {
	model             chat.Model
	streamer          chat.Streamer
	inputTokenCounter inputTokenCounter
	defaults          chat.Options
}

func New(model chat.Model, config Config) (*Client, error) {
	if lo.IsNil(model) {
		return nil, ErrNilModel
	}

	config, err := config.snapshot()
	if err != nil {
		return nil, err
	}

	streamer := config.Streamer
	if streamer == nil {
		streamer, _ = model.(chat.Streamer)
	}
	model = chat.Wrap(model, config.CallMiddleware...)
	if lo.IsNil(model) {
		return nil, errors.New("chatclient: call middleware returned a nil model")
	}
	inputTokenCounter, _ := model.(inputTokenCounter)
	if streamer != nil {
		streamer = chat.WrapStream(streamer, config.StreamMiddleware...)
		if lo.IsNil(streamer) {
			return nil, errors.New("chatclient: stream middleware returned a nil streamer")
		}
	}

	return &Client{
		model:             model,
		streamer:          streamer,
		inputTokenCounter: inputTokenCounter,
		defaults:          config.Defaults,
	}, nil
}

func (c *Client) Output[T any](format OutputFormat[T]) Generation[T] {
	return Generation[T]{client: c, format: format}
}

func (c *Client) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	if c == nil {
		return nil, ErrNilClient
	}
	return c.call(ctx, req, nil)
}

// SupportsInputTokenCounting reports whether CountInputTokens observes the same
// prepared provider request as Call. Middleware must explicitly preserve that
// capability after any request transformation.
func (c *Client) SupportsInputTokenCounting() bool {
	return c != nil && c.inputTokenCounter != nil
}

// CountInputTokens snapshots and resolves defaults exactly like Call, then asks
// the model's optional provider-owned counter to measure the complete input.
func (c *Client) CountInputTokens(ctx context.Context, req *chat.Request) (int64, error) {
	if c == nil {
		return 0, ErrNilClient
	}
	prepared, err := c.prepareRequest(req, nil)
	if err != nil {
		return 0, err
	}
	if c.inputTokenCounter == nil {
		return 0, ErrInputTokenCountingUnsupported
	}
	count, err := c.inputTokenCounter.CountInputTokens(ctx, prepared)
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, fmt.Errorf("chatclient: input token counter returned a negative count: %d", count)
	}
	return count, nil
}

func (c *Client) call(ctx context.Context, req *chat.Request, format *chat.OutputFormat) (*chat.Response, error) {
	prepared, err := c.prepareRequest(req, format)
	if err != nil {
		return nil, err
	}
	return c.model.Call(ctx, prepared)
}

func (c *Client) prepareRequest(request *chat.Request, outputFormat *chat.OutputFormat) (*chat.Request, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: nil request", chat.ErrInvalidRequest)
	}
	if outputFormat != nil && request.Options.OutputFormat != nil {
		return nil, fmt.Errorf("%w: request options already define output_format", ErrInvalidOutputFormat)
	}
	prepared := request.Clone()
	effectiveOptions, err := c.defaults.Resolve(request.Options)
	if err != nil {
		return nil, err
	}
	if outputFormat != nil {
		effectiveOptions.OutputFormat = outputFormat.Clone()
	}
	prepared.Options = effectiveOptions
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (c *Client) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	if c == nil {
		return errorSequence(ErrNilClient)
	}
	return c.stream(ctx, req, nil)
}

func (c *Client) stream(ctx context.Context, req *chat.Request, format *chat.OutputFormat) iter.Seq2[*chat.Response, error] {
	prepared, err := c.prepareRequest(req, format)
	if err != nil {
		return errorSequence(err)
	}
	if c.streamer == nil {
		return errorSequence(ErrStreamingUnsupported)
	}

	return func(yield func(*chat.Response, error) bool) {
		sequence := c.streamer.Stream(ctx, prepared)
		if sequence == nil {
			yield(nil, errNilStreamSequence)
			return
		}
		sequence(yield)
	}
}

func errorSequence(err error) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		yield(nil, err)
	}
}
