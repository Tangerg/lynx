package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// sentinelSpecs maps each protocol sentinel to its wire behavior (API.md §8.2).
// errors.Is checks identity, so iteration order is irrelevant. The symbolic
// ProblemData.type is the sentinel's Error string; clients branch on it, not
// on the numeric code.
type rpcErrorSpec struct {
	code              int
	retryable         bool
	retryAfterSeconds int
}

var sentinelSpecs = map[error]rpcErrorSpec{
	protocol.ErrMethodNotFound:         {code: protocol.CodeMethodNotFound},
	protocol.ErrInvalidParams:          {code: protocol.CodeInvalidParams},
	protocol.ErrProviderError:          {code: protocol.CodeProviderError},
	protocol.ErrSessionNotFound:        {code: protocol.CodeSessionNotFound},
	protocol.ErrRunNotFound:            {code: protocol.CodeRunNotFound},
	protocol.ErrItemNotFound:           {code: protocol.CodeItemNotFound},
	protocol.ErrCwdUnavailable:         {code: protocol.CodeCwdUnavailable},
	protocol.ErrCapabilityNotNeg:       {code: protocol.CodeCapabilityNotNeg},
	protocol.ErrRunAlreadyDone:         {code: protocol.CodeRunAlreadyDone},
	protocol.ErrCheckpointUnavailable:  {code: protocol.CodeCheckpointUnavail},
	protocol.ErrUnsupportedMime:        {code: protocol.CodeUnsupportedMime},
	protocol.ErrPathOutsideRoot:        {code: protocol.CodePathOutsideRoot},
	protocol.ErrInterruptNotOpen:       {code: protocol.CodeInterruptNotOpen},
	protocol.ErrInvalidProtocolVersion: {code: protocol.CodeInvalidProtocolVersion},
	protocol.ErrVcsUnavailable:         {code: protocol.CodeVcsUnavailable},
	protocol.ErrSessionBusy:            {code: protocol.CodeSessionBusy},
	protocol.ErrRevisionConflict:       {code: protocol.CodeRevisionConflict},
	protocol.ErrIdempotencyConflict:    {code: protocol.CodeIdempotencyConflict},
	protocol.ErrIdempotencyInProgress: {
		code: protocol.CodeIdempotencyInProgress, retryable: true, retryAfterSeconds: 1,
	},
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
	for sentinel, spec := range sentinelSpecs {
		if errors.Is(err, sentinel) {
			return problemErrorWithSpec(spec, sentinel.Error(), err.Error())
		}
	}
	return problemError(protocol.CodeInternalError, protocol.ProblemInternalError, "the runtime could not complete the request")
}

// problemError builds an Error carrying a ProblemData{type, detail}.
// typ is the symbolic name (API.md §8.2); detail is the human string.
func problemError(code int, typ, detail string) *transport.Error {
	return problemErrorWithSpec(rpcErrorSpec{code: code}, typ, detail)
}

func problemErrorWithSpec(spec rpcErrorSpec, typ, detail string) *transport.Error {
	// channel "rpc": every numeric-coded error is a synchronous JSON-RPC
	// error response (API.md §8.1 channel a).
	data, _ := json.Marshal(protocol.ProblemData{
		Type: typ, Channel: protocol.ErrorChannelRPC, Detail: detail,
		Retryable: spec.retryable, RetryAfterSeconds: spec.retryAfterSeconds,
	})
	return transport.NewError(spec.code, typ, data)
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

// badEnvelope is returned for malformed JSON-RPC envelopes (non-string
// id, wrong shape) at the dispatcher boundary.
func badEnvelope(detail string) *transport.Error {
	return problemError(protocol.CodeInvalidRequest, "invalid_request", detail)
}
