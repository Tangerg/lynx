package worker

import (
	"context"
	"github.com/Tangerg/lynx/core/msg"
)

type Worker interface {
	Sleep()
	Work(ctx context.Context, msg *msg.Msg) ([]*msg.Msg, error)
}
