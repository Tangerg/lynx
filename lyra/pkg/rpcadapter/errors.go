package rpcadapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// errorToRPC maps a Go error returned from CoreAPI into an RPCError
// envelope. Sentinel errors from pkg/coreapi map to their
// corresponding wire codes; everything else falls through to
// -32603 internal_error with the error's String() as detail.
func errorToRPC(err error) *transport.RPCError {
	if err == nil {
		return nil
	}
	if rpcErr := new(transport.RPCError); errors.As(err, &rpcErr) {
		return rpcErr
	}

	switch {
	case errors.Is(err, coreapi.ErrNotImplemented):
		return transport.NewError(transport.CodeMethodNotFound, nil)
	case errors.Is(err, coreapi.ErrSessionNotFound):
		return transport.NewError(transport.CodeSessionNotFound, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrMessageNotFound):
		return transport.NewError(transport.CodeMessageNotFound, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrRunNotFound):
		return transport.NewError(transport.CodeRunNotFound, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrAttachmentTooLarge):
		return transport.NewError(transport.CodeAttachmentTooLarge, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrCapabilityNotNegotiated):
		return transport.NewError(transport.CodeCapabilityNotNegotiated, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrInvalidProtocolVersion):
		return transport.NewError(transport.CodeInvalidProtocolVersion, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrProtocolViolation):
		return transport.NewError(transport.CodeProtocolViolation, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrApprovalRequired):
		return transport.NewError(transport.CodeApprovalRequired, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrProviderError):
		return transport.NewError(transport.CodeProviderError, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrProviderRateLimited):
		return transport.NewError(transport.CodeProviderRateLimited, problemDataFrom(err))
	case errors.Is(err, coreapi.ErrToolFailed):
		return transport.NewError(transport.CodeToolFailed, problemDataFrom(err))
	default:
		return transport.NewError(transport.CodeInternalError, problemDataFrom(err))
	}
}

// problemDataFrom packages the error string as a ProblemData detail —
// keeps the wire shape uniform across error classes.
func problemDataFrom(err error) json.RawMessage {
	if err == nil {
		return nil
	}
	data, mErr := json.Marshal(transport.ProblemData{Detail: err.Error()})
	if mErr != nil {
		return nil
	}
	return data
}

// invalidParams wraps an unmarshalling error as a -32602 invalid_params
// envelope. The detail surfaces the field/message that failed.
func invalidParams(reason string) *transport.RPCError {
	data, _ := json.Marshal(transport.ProblemData{Detail: reason})
	return transport.NewError(transport.CodeInvalidParams, data)
}

// methodNotFound builds the canonical -32601 envelope for an unknown
// method.
func methodNotFound(method string) *transport.RPCError {
	data, _ := json.Marshal(transport.ProblemData{Detail: fmt.Sprintf("unknown method %q", method)})
	return transport.NewError(transport.CodeMethodNotFound, data)
}

// protocolViolation is the -32011 envelope used when the client
// breaks a wire rule (calling a business method before initialize,
// URL/body method mismatch, ...).
func protocolViolation(detail string) *transport.RPCError {
	data, _ := json.Marshal(transport.ProblemData{Detail: detail})
	return transport.NewError(transport.CodeProtocolViolation, data)
}
