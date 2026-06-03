package dispatch

import (
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// This file holds the generic plumbing every business handler shares:
// decode the typed params, then map a Runtime method's (result, error)
// tail onto a HandleResult. Each handler keeps its own identity — which
// request type, which Runtime method, which required-field guards — but
// the repeated decode / error-map / encode spine lives here. A change to
// error mapping or the reply envelope touches one place, and no handler
// can forget to run [errorToRPC] or pick the wrong error code.

// decode unmarshals the typed request params. Empty params yield the zero
// value (valid for methods whose fields are all optional); malformed
// params map to invalid_params. The returned *transport.Error is nil on
// success. List/query methods that tolerate garbage params decode
// leniently with a bare [unmarshal] instead.
func decode[In any](msg *transport.Request) (In, *transport.Error) {
	var in In
	if err := unmarshal(msg.Params, &in); err != nil {
		return in, invalidParams(err.Error())
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

// replyStream maps a streaming method's (result, events, error) tail onto a
// HandleResult carrying the run id + event channel for the transport's
// notification pump. runID is read from the result only on success.
func replyStream[Out any](msg *transport.Request, out Out, runID string, events <-chan protocol.RunEvent, err error) HandleResult {
	if err != nil {
		return responseError(msg.ID, errorToRPC(err))
	}
	return streamingResult(msg.ID, out, runID, events)
}
