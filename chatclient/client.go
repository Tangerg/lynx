package chatclient

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/lynx/core/chat"
)

var (
	// ErrNilModel reports that New was called without a synchronous model.
	ErrNilModel = errors.New("chatclient: nil model")
	// ErrStreamingUnsupported reports that a Client has no streaming
	// capability. Pass a model that also implements [chat.Streamer], or use
	// [WithStreamer] when call and stream capabilities are separate values.
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
// [chat.Streamer], Stream uses that capability automatically. Functional
// options are reserved for construction-time defaults and composition; a
// request itself remains an ordinary [chat.Request].
func New(model chat.Model, options ...Option) (*Client, error) {
	if model == nil {
		return nil, ErrNilModel
	}

	cfg := config{}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("chatclient: nil option")
		}
		if err := option.apply(&cfg); err != nil {
			return nil, err
		}
	}
	if err := cfg.defaults.Validate(); err != nil {
		return nil, err
	}

	streamer := cfg.streamer
	if streamer == nil {
		streamer, _ = model.(chat.Streamer)
	}

	model = chat.Wrap(model, cfg.callMiddleware...)
	if model == nil {
		return nil, errors.New("chatclient: call middleware returned a nil model")
	}
	if streamer != nil {
		streamer = chat.WrapStream(streamer, cfg.streamMiddleware...)
		if streamer == nil {
			return nil, errors.New("chatclient: stream middleware returned a nil streamer")
		}
	}

	return &Client{
		model:    model,
		streamer: streamer,
		defaults: cfg.defaults.Clone(),
	}, nil
}

// Call snapshots and validates req, applies client defaults to fields the
// request leaves unspecified, and invokes the synchronous model capability.
func (c *Client) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	prepared, err := prepareRequest(req, c.defaults)
	if err != nil {
		return nil, err
	}
	return c.model.Call(ctx, prepared)
}

// Stream snapshots and validates req, applies client defaults, and returns a
// lazy response sequence. If the client has no real streaming capability, the
// sequence yields (nil, ErrStreamingUnsupported) once and terminates.
func (c *Client) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	prepared, err := prepareRequest(req, c.defaults)
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
