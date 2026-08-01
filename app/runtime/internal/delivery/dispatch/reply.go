package dispatch

import (
	"iter"

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
	if err := protocol.ValidateWireTree(result); err != nil {
		return responseError(id, problemError(
			protocol.ErrInternalError,
			"the runtime produced an invalid response",
		))
	}
	resp, err := transport.NewResponseResult(id, result)
	if err != nil {
		// Response encoding is an infrastructure fault. Its serializer detail can
		// contain arbitrary implementation data, so the protocol exposes only a
		// stable client-safe problem.
		return responseError(id, problemError(protocol.ErrInternalError, "the runtime could not encode the response"))
	}
	return HandleResult{Response: resp}
}

func responseError(id transport.ID, rpcErr *transport.Error) HandleResult {
	return HandleResult{Response: transport.NewResponseError(id, rpcErr)}
}

// streamingResult attaches the frame sequence onto the synchronous reply;
// the transport streams it as the call's own response (streamable HTTP).
func streamingResult(id transport.ID, result any, events iter.Seq[StreamFrame]) HandleResult {
	res := responseResult(id, result)
	res.EventStream = events
	return res
}

// decode validates and unmarshals typed request params. Empty params yield the
// zero value for methods whose fields are all optional. Present params must be
// one JSON object whose fields are known by the request DTO; malformed, null,
// or drifted requests fail at the delivery boundary instead of silently
// discarding client intent.
//
// A request whose type states its own constraints ([protocol.WireValidator]) is
// checked here — once, on the one path every method's params travel. That is why
// no handler re-checks a required field: a second check is a second author, and
// the one that gets forgotten is the one that matters.
func decode[In any](msg *transport.Request) (In, *transport.Error) {
	var in In
	if err := decodeParams(msg.Params, &in); err != nil {
		return in, invalidParams(err.Error())
	}
	if err := protocol.ValidateWireTree(&in); err != nil {
		return in, invalidRequestShape(err)
	}
	return in, nil
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
