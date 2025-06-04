package model

import "context"

type ToolContext struct {
	ctx context.Context
}

func (t *ToolContext) Context() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}
