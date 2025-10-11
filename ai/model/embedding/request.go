package embedding

import (
	"errors"
)

type Request struct {
	Inputs  []string
	Options Options
	Params  map[string]any //context params
}

func NewRequest(inputs []string) (*Request, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one string is required")
	}

	return &Request{
		Inputs: inputs,
		Params: make(map[string]any),
	}, nil
}

// ensureExtra initializes the params map if it hasn't been
// created yet to prevent nil pointer operations.
func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

// Get retrieves a parameter value by key.
// Returns the value and true if found, or nil and false otherwise.
func (r *Request) Get(key string) (any, bool) {
	r.ensureParams()
	val, ok := r.Params[key]
	return val, ok
}

// Set stores a parameter value with the specified key.
// Automatically initializes the params map if needed.
func (r *Request) Set(key string, val any) {
	r.ensureParams()
	r.Params[key] = val
}
