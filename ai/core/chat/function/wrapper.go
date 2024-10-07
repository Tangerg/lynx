package function

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/core/model/function"
)

var _ function.Function = (*Wrapper)(nil)

// Wrapper is a struct that encapsulates a function with additional metadata.
// It includes a name, description, input type schema, and a caller function.
type Wrapper struct {
	name            string
	description     string
	inputTypeSchema string
	caller          func(ctx context.Context, input string) (string, error)
}

func (w *Wrapper) Name() string {
	return w.name
}

func (w *Wrapper) Description() string {
	return w.description
}

func (w *Wrapper) InputTypeSchema() string {
	return w.inputTypeSchema
}

func (w *Wrapper) Call(ctx context.Context, input string) (string, error) {
	return w.caller(ctx, input)
}

type WrapperBuilder struct {
	wrapper *Wrapper
}

func NewWrapperBuilder() *WrapperBuilder {
	return &WrapperBuilder{
		wrapper: new(Wrapper),
	}
}
func (w *WrapperBuilder) WithName(name string) *WrapperBuilder {
	w.wrapper.name = name
	return w
}
func (w *WrapperBuilder) WithDescription(desc string) *WrapperBuilder {
	w.wrapper.description = desc
	return w
}
func (w *WrapperBuilder) WithInputTypeSchema(schema string) *WrapperBuilder {
	w.wrapper.inputTypeSchema = schema
	return w
}
func (w *WrapperBuilder) WithCaller(caller func(ctx context.Context, input string) (string, error)) *WrapperBuilder {
	w.wrapper.caller = caller
	return w
}
func (w *WrapperBuilder) Build() (*Wrapper, error) {
	if w.wrapper.name == "" {
		return nil, errors.New("name can not empty")
	}
	if w.wrapper.inputTypeSchema == "" {
		return nil, errors.New("input type schema can not empty")
	}
	if w.wrapper.caller == nil {
		return nil, errors.New("caller can not nil")
	}
	return w.wrapper, nil
}
