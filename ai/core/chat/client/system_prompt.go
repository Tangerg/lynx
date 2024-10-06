package client

import (
	"strings"
)

// SystemPrompt is an interface that defines the contract for managing system prompts
// and their associated parameters in a chat application. A system prompt typically includes
// instructions or context provided by the system to guide the chat interaction.
//
// Methods:
//
// Text() string
//   - Returns the text of the system prompt.
//   - This method provides access to the current system prompt text used in the chat application.
//
// Param(key string) (any, bool)
//   - Retrieves a parameter value associated with the specified key.
//   - Returns the value and a boolean indicating whether the key was found in the parameters map.
//
// Params() map[string]any
//   - Returns a map of all parameters currently set for the system prompt.
//   - This method provides access to all key-value pairs used to configure the system prompt.
//
// SetText(text string) SystemPrompt
//   - Sets the text of the system prompt.
//   - Returns the SystemPrompt instance to allow method chaining.
//
// SetParam(k string, v any) SystemPrompt
//   - Sets a single parameter key-value pair for the system prompt configuration.
//   - Returns the SystemPrompt instance to allow method chaining.
//
// SetParams(m map[string]any) SystemPrompt
//   - Sets multiple parameters using a map of key-value pairs for the system prompt configuration.
//   - Returns the SystemPrompt instance to allow method chaining.
type SystemPrompt interface {
	Text() string
	Param(key string) (any, bool)
	Params() map[string]any
	SetText(text string) SystemPrompt
	SetParam(k string, v any) SystemPrompt
	SetParams(m map[string]any) SystemPrompt
}

func NewDefaultSystemPrompt() *DefaultSystemPrompt {
	return &DefaultSystemPrompt{
		params: make(map[string]any),
	}
}

var _ SystemPrompt = (*DefaultSystemPrompt)(nil)

type DefaultSystemPrompt struct {
	text   string
	params map[string]any
}

func (d *DefaultSystemPrompt) Text() string {
	return d.text
}

func (d *DefaultSystemPrompt) Param(key string) (any, bool) {
	v, ok := d.params[key]
	return v, ok
}

func (d *DefaultSystemPrompt) Params() map[string]any {
	return d.params
}

func (d *DefaultSystemPrompt) SetText(text string) SystemPrompt {
	d.text = strings.TrimSpace(text)
	return d
}

func (d *DefaultSystemPrompt) SetParam(k string, v any) SystemPrompt {
	d.params[k] = v
	return d
}

func (d *DefaultSystemPrompt) SetParams(m map[string]any) SystemPrompt {
	for k, v := range m {
		d.params[k] = v
	}
	return d
}
