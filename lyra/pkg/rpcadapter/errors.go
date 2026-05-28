package rpcadapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// sentinelToCode maps each coreapi sentinel error to its wire code.
// Iteration order doesn't matter — errors.Is checks identity, so at
// most one entry matches per call. Adding a new sentinel = one line
// here, no switch case to edit.
var sentinelToCode = map[error]int{
	coreapi.ErrSessionNotFound:         transport.CodeSessionNotFound,
	coreapi.ErrMessageNotFound:         transport.CodeMessageNotFound,
	coreapi.ErrRunNotFound:             transport.CodeRunNotFound,
	coreapi.ErrAttachmentTooLarge:      transport.CodeAttachmentTooLarge,
	coreapi.ErrCapabilityNotNegotiated: transport.CodeCapabilityNotNegotiated,
	coreapi.ErrInvalidProtocolVersion:  transport.CodeInvalidProtocolVersion,
	coreapi.ErrProtocolViolation:       transport.CodeProtocolViolation,
	coreapi.ErrApprovalRequired:        transport.CodeApprovalRequired,
	coreapi.ErrProviderError:           transport.CodeProviderError,
	coreapi.ErrProviderRateLimited:     transport.CodeProviderRateLimited,
	coreapi.ErrToolFailed:              transport.CodeToolFailed,
}

// errorToRPC maps a Go error returned from Runtime into a JSON-RPC
// Error envelope. Resolution order:
//
//  1. If err already wraps a *transport.Error, surface it verbatim
//     (preserves any custom message / code the impl chose).
//  2. ErrNotImplemented → -32601 method_not_found, no problem data
//     (the wire shouldn't leak internal "not implemented" text).
//  3. Sentinel match in [sentinelToCode] → corresponding code +
//     problem-data detail.
//  4. Anything else → -32603 internal_error + detail.
func errorToRPC(err error) *transport.Error {
	if err == nil {
		return nil
	}
	var rpcErr *transport.Error
	if errors.As(err, &rpcErr) {
		return rpcErr
	}
	if errors.Is(err, coreapi.ErrNotImplemented) {
		return transport.NewError(transport.CodeMethodNotFound, nil)
	}
	for sentinel, code := range sentinelToCode {
		if errors.Is(err, sentinel) {
			return transport.NewError(code, problemDataFrom(err))
		}
	}
	return transport.NewError(transport.CodeInternalError, problemDataFrom(err))
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

// problemDetail is a one-shot helper for the three "build envelope
// from a single detail string" helpers below.
func problemDetail(detail string) json.RawMessage {
	data, _ := json.Marshal(transport.ProblemData{Detail: detail})
	return data
}

// invalidParams wraps an unmarshalling error as a -32602 invalid_params
// envelope. The detail surfaces the field/message that failed.
func invalidParams(reason string) *transport.Error {
	return transport.NewError(transport.CodeInvalidParams, problemDetail(reason))
}

// methodNotFound builds the canonical -32601 envelope for an unknown
// method.
func methodNotFound(method string) *transport.Error {
	return transport.NewError(transport.CodeMethodNotFound,
		problemDetail(fmt.Sprintf("unknown method %q", method)))
}

// protocolViolation is the -32011 envelope used when the client
// breaks a wire rule (calling a business method before initialize,
// URL/body method mismatch, ...).
func protocolViolation(detail string) *transport.Error {
	return transport.NewError(transport.CodeProtocolViolation, problemDetail(detail))
}
