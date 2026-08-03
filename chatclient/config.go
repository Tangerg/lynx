package chatclient

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
)

// Config describes construction-time Client behavior. Request-specific
// values belong in chat.Request.
//
// The slices and Defaults are snapshotted by New, so callers may safely reuse
// or mutate their input after construction. The first middleware in each
// slice is the outermost wrapper, matching [chat.Wrap] and [chat.WrapStream].
type Config struct {
	Defaults         chat.Options
	Streamer         chat.Streamer
	CallMiddleware   []chat.CallMiddleware
	StreamMiddleware []chat.StreamMiddleware
}

func (c Config) snapshot() (Config, error) {
	c.Defaults = c.Defaults.Clone()
	if err := c.Defaults.Validate(); err != nil {
		return Config{}, err
	}
	if c.Streamer != nil && isNil(c.Streamer) {
		return Config{}, errors.New("chatclient: nil streamer")
	}
	c.CallMiddleware = slices.Clone(c.CallMiddleware)
	c.StreamMiddleware = slices.Clone(c.StreamMiddleware)
	return c, nil
}
