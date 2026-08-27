package chatclient

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/samber/lo"
)

var (
	// ErrNilModel reports that New was called without a synchronous model.
	ErrNilModel = errors.New("chatclient: nil model")
	// ErrNilClient reports an operation created without a Client.
	ErrNilClient = errors.New("chatclient: nil client")
	// ErrStreamingUnsupported reports that a Client has no streaming
	// capability. Pass a model that also implements [chat.Streamer], or set
	// [Config.Streamer] when call and stream capabilities are separate values.
	ErrStreamingUnsupported = errors.New("chatclient: streaming unsupported")
)

var errNilStreamSequence = errors.New("chatclient: streamer returned a nil sequence")

// Client is an immutable, concurrency-safe composition of chat capabilities,
// defaults, and middleware. It does not make an underlying model concurrency
// safe; callers must still follow the model's concurrency contract.
//
// Call and Stream accept ordinary [chat.Request] values directly. Client
// snapshots each request before invoking middleware or a provider, so those
// layers cannot mutate caller-owned protocol values.
type Client struct {
	model    chat.Model
	streamer chat.Streamer
	defaults chat.Options
}

// New constructs a Client around model. When model also implements
// [chat.Streamer], Stream uses that capability automatically unless config
// supplies a separate streaming capability.
func New(model chat.Model, config Config) (*Client, error) {
	if lo.IsNil(model) {
		return nil, ErrNilModel
	}

	cfg, err := config.snapshot()
	if err != nil {
		return nil, err
	}

	streamer := cfg.Streamer
	if streamer == nil {
		streamer, _ = model.(chat.Streamer)
	}

	model = chat.Wrap(model, cfg.CallMiddleware...)
	if lo.IsNil(model) {
		return nil, errors.New("chatclient: call middleware returned a nil model")
	}
	if streamer != nil {
		streamer = chat.WrapStream(streamer, cfg.StreamMiddleware...)
		if lo.IsNil(streamer) {
			return nil, errors.New("chatclient: stream middleware returned a nil streamer")
		}
	}

	return &Client{
		model:    model,
		streamer: streamer,
		defaults: cfg.Defaults,
	}, nil
}

// Output binds format to a typed generation without modifying c.
func (c *Client) Output[T any](format OutputFormat[T]) Generation[T] {
	return Generation[T]{client: c, format: format}
}

// Call snapshots and validates req, applies client defaults to fields the
// request leaves unspecified, and invokes the synchronous model capability.
func (c *Client) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	if c == nil {
		return nil, ErrNilClient
	}
	return c.call(ctx, req, nil)
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
	merged, err := c.defaults.Merged(request.Options)
	if err != nil {
		return nil, err
	}
	if outputFormat != nil {
		merged.OutputFormat = outputFormat.Clone()
	}
	prepared.Options = merged
	if err := prepared.Validate(); err != nil {
		return nil, err
	}
	return prepared, nil
}

// Stream snapshots and validates req, applies client defaults, and returns a
// lazy response sequence. If the client has no real streaming capability, the
// sequence yields (nil, ErrStreamingUnsupported) once and terminates.
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
