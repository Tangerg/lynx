package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// sentinelToCode maps each protocol sentinel error to its numeric wire
// code (API.md §8.2). errors.Is checks identity, so at most one entry
// matches per call — iteration order is irrelevant. Adding a sentinel =
// one line here. The symbolic type carried in ProblemData.type is the
// sentinel's Error() string (clients judge by type, not by code).
var sentinelToCode = map[error]int{
	protocol.ErrMethodNotFound:         protocol.CodeMethodNotFound,
	protocol.ErrInvalidParams:          protocol.CodeInvalidParams,
	protocol.ErrProviderError:          protocol.CodeProviderError,
	protocol.ErrSessionNotFound:        protocol.CodeSessionNotFound,
	protocol.ErrRunNotFound:            protocol.CodeRunNotFound,
	protocol.ErrItemNotFound:           protocol.CodeItemNotFound,
	protocol.ErrCwdUnavailable:         protocol.CodeCwdUnavailable,
	protocol.ErrCapabilityNotNeg:       protocol.CodeCapabilityNotNeg,
	protocol.ErrRunNotRunning:          protocol.CodeRunNotRunning,
	protocol.ErrRunAlreadyDone:         protocol.CodeRunAlreadyDone,
	protocol.ErrCheckpointUnavailable:  protocol.CodeCheckpointUnavail,
	protocol.ErrAttachmentTooLarge:     protocol.CodeAttachmentTooLarge,
	protocol.ErrUnsupportedMime:        protocol.CodeUnsupportedMime,
	protocol.ErrToolDenied:             protocol.CodeToolDenied,
	protocol.ErrPathOutsideRoot:        protocol.CodePathOutsideRoot,
	protocol.ErrInterruptNotOpen:       protocol.CodeInterruptNotOpen,
	protocol.ErrIdempotencyConflict:    protocol.CodeIdempotencyConflict,
	protocol.ErrInvalidProtocolVersion: protocol.CodeInvalidProtocolVersion,
}

// errorToRPC maps a Go error returned from Runtime into a JSON-RPC
// Error envelope. Resolution order:
//
//  1. An already-wrapped *transport.Error surfaces verbatim.
//  2. A sentinel match → its code + ProblemData{type, detail}.
//  3. Anything else → internal_error + detail.
//
// The wire message is the symbolic type so logs/traces read cleanly;
// clients branch on error.data.type, not the numeric code (API.md §8.2).
func errorToRPC(err error) *transport.Error {
	if err == nil {
		return nil
	}
	if rpcErr, ok := errors.AsType[*transport.Error](err); ok {
		return rpcErr
	}
	for sentinel, code := range sentinelToCode {
		if errors.Is(err, sentinel) {
			return problemError(code, sentinel.Error(), err.Error())
		}
	}
	return problemError(protocol.CodeInternalError, "internal_error", err.Error())
}

// problemError builds an Error carrying a ProblemData{type, detail}.
// typ is the symbolic name (API.md §8.2); detail is the human string.
func problemError(code int, typ, detail string) *transport.Error {
	data, _ := json.Marshal(protocol.ProblemData{Type: typ, Detail: detail})
	return transport.NewErrorWithMessage(code, typ, data)
}

// invalidParams wraps a params-validation failure as invalid_params.
func invalidParams(reason string) *transport.Error {
	return problemError(protocol.CodeInvalidParams, "invalid_params", reason)
}

// methodNotFound is the canonical envelope for an unknown method.
func methodNotFound(method string) *transport.Error {
	return problemError(protocol.CodeMethodNotFound, "method_not_found",
		fmt.Sprintf("unknown method %q", method))
}

// notInitialized is returned when a business method is called before
// runtime.initialize has succeeded (API.md §7.1 handshake gate).
func notInitialized(detail string) *transport.Error {
	return problemError(protocol.CodeCapabilityNotNeg, "capability_not_negotiated", detail)
}

// badEnvelope is returned for malformed JSON-RPC envelopes (non-string
// id, wrong shape) at the dispatcher boundary.
func badEnvelope(detail string) *transport.Error {
	return problemError(protocol.CodeInvalidRequest, "invalid_request", detail)
}
