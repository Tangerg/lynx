package chatclient

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

// Option configures construction-time Client behavior. Request-specific
// values belong in chat.Request rather than in options or fluent builders.
type Option interface {
	apply(*config) error
}

type optionFunc func(*config) error

func (f optionFunc) apply(cfg *config) error {
	return f(cfg)
}

type config struct {
	defaults         chat.Options
	streamer         chat.Streamer
	callMiddleware   []chat.CallMiddleware
	streamMiddleware []chat.StreamMiddleware
}

// WithDefaults sets provider-neutral client defaults. A request overrides
// each non-zero scalar or non-nil pointer/slice field. A non-nil empty Stop
// slice explicitly clears a client default.
func WithDefaults(defaults chat.Options) Option {
	snapshot := cloneOptions(defaults)
	return optionFunc(func(cfg *config) error {
		if err := snapshot.Validate(); err != nil {
			return err
		}
		cfg.defaults = cloneOptions(snapshot)
		return nil
	})
}

// WithStreamer supplies a streaming capability separate from the synchronous
// model passed to New. It takes precedence over a Streamer implemented by the
// model itself.
func WithStreamer(streamer chat.Streamer) Option {
	return optionFunc(func(cfg *config) error {
		if streamer == nil {
			return errors.New("chatclient: nil streamer")
		}
		cfg.streamer = streamer
		return nil
	})
}

// WithCallMiddleware appends synchronous middleware. The first middleware is
// the outermost wrapper, matching [chat.Wrap]. Nil entries are ignored.
func WithCallMiddleware(middleware ...chat.CallMiddleware) Option {
	snapshot := slices.Clone(middleware)
	return optionFunc(func(cfg *config) error {
		cfg.callMiddleware = append(cfg.callMiddleware, snapshot...)
		return nil
	})
}

// WithStreamMiddleware appends streaming middleware. The first middleware is
// the outermost wrapper, matching [chat.WrapStream]. Nil entries are ignored.
func WithStreamMiddleware(middleware ...chat.StreamMiddleware) Option {
	snapshot := slices.Clone(middleware)
	return optionFunc(func(cfg *config) error {
		cfg.streamMiddleware = append(cfg.streamMiddleware, snapshot...)
		return nil
	})
}
