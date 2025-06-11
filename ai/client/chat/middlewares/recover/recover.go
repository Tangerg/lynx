package recover

import (
	"errors"
	"github.com/Tangerg/lynx/ai/client/chat"
	"github.com/Tangerg/lynx/pkg/safe"
	"github.com/Tangerg/lynx/pkg/stream"
)

type recoverer struct{}

func newRecoverer() *recoverer {
	return &recoverer{}
}

func (r *recoverer) callMiddleware(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(request *chat.Request) (resp *chat.Response, err error) {
		safe.WithRecover(func() {
			resp, err = next.Call(request)
		}, func(panicErr error) {
			err = errors.Join(err, panicErr)
		})()
		return
	})
}

func (r *recoverer) streamMiddleware(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(request *chat.Request) (resp stream.Reader[*chat.Response], err error) {
		safe.WithRecover(func() {
			resp, err = next.Stream(request)
		}, func(panicErr error) {
			err = errors.Join(err, panicErr)
		})()
		return
	})
}

func New() (chat.CallMiddleware, chat.StreamMiddleware) {
	r := newRecoverer()
	return r.callMiddleware, r.streamMiddleware
}
