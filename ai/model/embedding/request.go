package embedding

import (
	"errors"
	"sync"

	"github.com/Tangerg/lynx/ai/model"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

var _ model.Request[[]string, Options] = (*request[Options])(nil)

type Request = request[Options]

type request[O Options] struct {
	inputs  []string
	options O
	mu      sync.RWMutex
	params  map[string]any //context params
}

func NewRequest(inputs []string, options ...Options) (*Request, error) {
	if len(inputs) == 0 {
		return nil, errors.New("at least one string is required")
	}

	return &request[Options]{
		inputs:  inputs,
		options: pkgSlices.FirstOr(options, nil),
		params:  make(map[string]any),
	}, nil
}

func (c *request[O]) Instructions() []string {
	return c.inputs
}

func (c *request[O]) Options() O {
	return c.options
}

func (c *request[O]) Params() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.params
}

func (c *request[O]) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	val, ok := c.params[key]
	return val, ok
}

func (c *request[O]) Set(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.params[key] = val
}

func (c *request[O]) SetParams(params map[string]any) {
	if params == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.params = params
}
