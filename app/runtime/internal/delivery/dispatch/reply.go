package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// This file holds the generic plumbing every business handler shares:
// decode the typed params, then map a Runtime method's (result, error)
// tail onto a HandleResult. Each handler keeps its own identity — which
// request type, which Runtime method, which required-field guards — but
// the repeated decode / error-map / encode spine lives here. A change to
// error mapping or the reply envelope touches one place, and no handler
// can forget to run [errorToRPC] or pick the wrong error code.

func responseResult(id transport.ID, result any) HandleResult {
	resp, err := transport.NewResponseResult(id, result)
	if err != nil {
		return responseError(id, problemError(protocol.CodeInternalError, protocol.ProblemInternalError, err.Error()))
	}
	return HandleResult{Response: resp}
}

func responseError(id transport.ID, rpcErr *transport.Error) HandleResult {
	return HandleResult{Response: transport.NewResponseError(id, rpcErr)}
}

// streamingResult attaches the frame channel onto the synchronous reply;
// the transport streams it as the call's own response (streamable HTTP).
func streamingResult(id transport.ID, result any, events <-chan StreamFrame) HandleResult {
	res := responseResult(id, result)
	res.EventStream = events
	return res
}

// decode validates and unmarshals typed request params. Empty params yield the
// zero value for methods whose fields are all optional. Present params must be
// one JSON object whose fields are known by the request DTO; malformed, null,
// or drifted requests fail at the delivery boundary instead of silently
// discarding client intent.
func decode[In any](msg *transport.Request) (In, *transport.Error) {
	var in In
	if err := decodeParams(msg.Params, &in); err != nil {
		return in, invalidParams(err.Error())
	}
	return in, nil
}

func decodeEmpty(msg *transport.Request) *transport.Error {
	_, bad := decode[struct{}](msg)
	return bad
}

// reply maps a (result, error) method tail onto a HandleResult: errors go
// through [errorToRPC], success encodes the result.
func reply[Out any](msg *transport.Request, out Out, err error) HandleResult {
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, out)
}

// replyDone is the ack-only tail: the method returns just an error and the
// successful reply is an empty object.
func replyDone(msg *transport.Request, err error) HandleResult {
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return responseResult(msg.ID, struct{}{})
}

// replyStream maps a streaming method's (result, events, error) tail onto
// a HandleResult carrying the synchronous reply + its event channel (the
// transport streams the channel as the call's own response).
func replyStream[Out any](ctx context.Context, msg *transport.Request, out Out, events <-chan protocol.RunEvent, err error) HandleResult {
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, adaptStream(ctx, events, runEventToFrameFor(ctx)))
}
