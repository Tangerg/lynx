package dispatch

import (
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

func responseResult(id transport.ID, result any) HandleResult {
	if err := protocol.ValidateWireTree(result); err != nil {
		return responseError(id, problemError(
			protocol.ErrInternalError,
			"the runtime produced an invalid response",
		))
	}
	response, err := transport.NewResponseResult(id, result)
	if err != nil {
		// Response encoding is an infrastructure fault. Its serializer detail can
		// contain arbitrary implementation data, so the protocol exposes only a
		// stable client-safe problem.
		return responseError(id, problemError(protocol.ErrInternalError, "the runtime could not encode the response"))
	}
	return HandleResult{Response: response}
}

func responseError(id transport.ID, rpcError *transport.Error) HandleResult {
	return HandleResult{Response: transport.NewResponseError(id, rpcError)}
}

// streamingResult attaches the frame sequence to the synchronous response;
// the transport streams it as the call's own response body.
func streamingResult(id transport.ID, result any, events iter.Seq[StreamFrame]) HandleResult {
	response := responseResult(id, result)
	response.EventStream = events
	return response
}

// reply maps a method's (result, error) pair onto a HandleResult: errors pass
// through [errorToRPC], while successful results are encoded and validated.
func reply[Result any](request *transport.Request, result Result, err error) HandleResult {
	if err != nil {
		return responseError(request.ID, errorToRPC(err))
	}
	return responseResult(request.ID, result)
}

// replyDone maps an acknowledgement-only method to an empty-object response.
func replyDone(request *transport.Request, err error) HandleResult {
	if err != nil {
		return responseError(request.ID, errorToRPC(err))
	}
	return responseResult(request.ID, struct{}{})
}
