package client

import (
	"github.com/Tangerg/lynx/ai/core/model/media"
	"strings"
)

// UserPrompt is an interface that defines the contract for managing user prompts
// and their associated parameters in a chat application. A user prompt typically includes
// the input or query provided by the user to initiate or continue a chat interaction.
//
// Methods:
//
// Text() string
//   - Returns the text of the user prompt.
//   - This method provides access to the current user prompt text used in the chat application.
//
// Param(key string) (any, bool)
//   - Retrieves a parameter value associated with the specified key.
//   - Returns the value and a boolean indicating whether the key was found in the parameters map.
//
// Params() map[string]any
//   - Returns a map of all parameters currently set for the user prompt.
//   - This method provides access to all key-value pairs used to configure the user prompt.
//
// SetText(text string) UserPrompt
//   - Sets the text of the user prompt.
//   - Returns the UserPrompt instance to allow method chaining.
//
// SetParam(k string, v any) UserPrompt
//   - Sets a single parameter key-value pair for the user prompt configuration.
//   - Returns the UserPrompt instance to allow method chaining.
//
// SetParams(m map[string]any) UserPrompt
//   - Sets multiple parameters using a map of key-value pairs for the user prompt configuration.
//   - Returns the UserPrompt instance to allow method chaining.
type UserPrompt interface {
	Text() string
	Param(key string) (any, bool)
	Params() map[string]any
	Media() []*media.Media
	SetText(text string) UserPrompt
	SetParam(k string, v any) UserPrompt
	SetParams(m map[string]any) UserPrompt
	SetMedia(media ...*media.Media) UserPrompt
}

func NewDefaultUserPrompt() *DefaultUserPrompt {
	return &DefaultUserPrompt{
		params: make(map[string]any),
	}
}

var _ UserPrompt = (*DefaultUserPrompt)(nil)

type DefaultUserPrompt struct {
	text   string
	params map[string]any
	media  []*media.Media
}

func (d *DefaultUserPrompt) Media() []*media.Media {
	return d.media
}

func (d *DefaultUserPrompt) Text() string {
	return d.text
}

func (d *DefaultUserPrompt) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultUserPrompt) Params() map[string]any {
	return d.params
}

func (d *DefaultUserPrompt) SetText(text string) UserPrompt {
	d.text = strings.TrimSpace(text)
	return d
}

func (d *DefaultUserPrompt) SetParam(k string, v any) UserPrompt {
	d.params[k] = v
	return d
}

func (d *DefaultUserPrompt) SetParams(m map[string]any) UserPrompt {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}

func (d *DefaultUserPrompt) SetMedia(media ...*media.Media) UserPrompt {
	d.media = append(d.media, media...)
	return d
}
