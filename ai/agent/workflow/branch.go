package workflow

import (
	"context"
)

var _ Node = (*Branch)(nil)

type Branch struct {
}

func (b *Branch) Name() string {
	return "branch"
}

func (b *Branch) Run(ctx context.Context, input State) (State, error) {
	//TODO implement me
	panic("implement me")
}
