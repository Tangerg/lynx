package dispatch

import (
	"github.com/Tangerg/scope/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

func responseResult(id transport.ID, result any) Result {
	response, err := transport.NewResponseResult(id, result)
	if err != nil {
		// Response encoding is an infrastructure fault. Its serializer detail can
		// contain arbitrary implementation data, so the protocol exposes only a
		// stable client-safe problem.
		return responseError(id, problemError(protocol.ErrInternalError, "the runtime could not encode the response"))
	}
	return Result{Response: response}
}

func responseError(id transport.ID, rpcError *transport.Error) Result {
	return Result{Response: transport.NewResponseError(id, rpcError)}
}
