package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

type rpcCodeSpec struct {
	problemType string
	code        int
}

var rpcCodeSpecs = mustRPCCodeSpecs([]rpcCodeSpec{
	{protocol.ErrInvalidRequest.Error(), codeInvalidRequest},
	{protocol.ErrInternalError.Error(), codeInternalError},
	{protocol.ErrMethodNotFound.Error(), codeMethodNotFound},
	{protocol.ErrInvalidParams.Error(), codeInvalidParams},
	{protocol.ErrSessionNotFound.Error(), codeSessionNotFound},
	{protocol.ErrRunNotFound.Error(), codeRunNotFound},
	{protocol.ErrItemNotFound.Error(), codeItemNotFound},
	{protocol.ErrMCPServerNotFound.Error(), codeMCPServerNotFound},
	{protocol.ErrMCPServerAlreadyExists.Error(), codeMCPServerExists},
	{protocol.ErrMCPServerDisabled.Error(), codeMCPServerDisabled},
	{protocol.ErrMCPAuthorizationAttemptNotFound.Error(), codeMCPAuthorizationAttemptNotFound},
	{protocol.ErrRunNotRoot.Error(), codeRunNotRoot},
	{protocol.ErrSessionBusy.Error(), codeSessionBusy},
	{protocol.ErrRevisionConflict.Error(), codeRevisionConflict},
	{protocol.ErrInterruptNotOpen.Error(), codeInterruptNotOpen},
	{protocol.ErrReplayUnavailable.Error(), codeReplayUnavailable},
	{protocol.ErrRunWaiting.Error(), codeRunWaiting},
	{protocol.ErrRunFinished.Error(), codeRunFinished},
	{protocol.ErrStaleSegment.Error(), codeStaleSegment},
	{protocol.ErrReplayCursorInvalid.Error(), codeReplayCursorInvalid},
	{protocol.ErrSessionHasActiveRun.Error(), codeSessionHasActiveRun},
	{protocol.ErrCapabilityNotNeg.Error(), codeCapabilityNotNeg},
	{protocol.ErrWorkspaceUnavailable.Error(), codeWorkspaceUnavailable},
	{protocol.ErrCheckpointUnavailable.Error(), codeCheckpointUnavail},
	{protocol.ErrUnsupportedMime.Error(), codeUnsupportedMime},
	{protocol.ErrPathOutsideRoot.Error(), codePathOutsideRoot},
	{protocol.ErrVcsUnavailable.Error(), codeVCSUnavailable},
	{protocol.ErrInvalidProtocolVersion.Error(), codeInvalidProtocolVersion},
	{protocol.ErrIdempotencyConflict.Error(), codeIdempotencyConflict},
	{protocol.ErrProviderError.Error(), codeProviderError},
	{protocol.ErrIdempotencyInProgress.Error(), codeIdempotencyInProgress},
	{protocol.ErrIdempotencyStoreMismatch.Error(), codeIdempotencyStoreMismatch},
})

func ProblemCodes() map[string]int {
	out := make(map[string]int, len(rpcCodeSpecs))
	for _, spec := range rpcCodeSpecs {
		out[spec.problemType] = spec.code
	}
	return out
}

func problemCode(problemType string) (int, bool) {
	for _, spec := range rpcCodeSpecs {
		if spec.problemType == problemType {
			return spec.code, true
		}
	}
	return 0, false
}

func errorToRPC(err error) *transport.Error {
	if err == nil {
		return nil
	}
	if rpcError, ok := errors.AsType[*transport.Error](err); ok {
		return rpcError
	}
	return marshalFailure(operation.ProjectError(err))
}

func marshalFailure(failure *operation.Failure) *transport.Error {
	if failure == nil {
		failure = operation.NewFailure(protocol.ErrInternalError, "the runtime could not complete the request")
	}
	problem := failure.Problem()
	code, ok := problemCode(problem.Type)
	if !ok || protocol.ValidateWireTree(problem) != nil {
		return invalidProblemResponse("the runtime could not encode a valid error response")
	}
	encoded, err := json.Marshal(problem)
	if err != nil {
		return invalidProblemResponse("the runtime could not serialize a valid error response")
	}
	return transport.NewError(code, problem.Type, encoded)
}

func problemError(sentinel error, detail string) *transport.Error {
	return marshalFailure(operation.NewFailure(sentinel, detail))
}

func invalidProblemResponse(detail string) *transport.Error {
	fallback := protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: detail}
	encoded, err := json.Marshal(fallback)
	if err != nil {
		return transport.NewError(codeInternalError, protocol.ProblemInternalError, nil)
	}
	return transport.NewError(codeInternalError, protocol.ProblemInternalError, encoded)
}

func invalidParams(reason string) *transport.Error {
	return problemError(protocol.ErrInvalidParams, reason)
}

func badEnvelope(detail string) *transport.Error {
	return problemError(protocol.ErrInvalidRequest, detail)
}

func mustRPCCodeSpecs(specs []rpcCodeSpec) []rpcCodeSpec {
	types := make(map[string]bool, len(specs))
	codes := make(map[int]bool, len(specs))
	for index, spec := range specs {
		switch {
		case spec.problemType == "":
			panic(fmt.Sprintf("dispatch: RPC code spec %d has no problem type", index))
		case types[spec.problemType]:
			panic(fmt.Sprintf("dispatch: RPC problem type %q is registered twice", spec.problemType))
		case codes[spec.code]:
			panic(fmt.Sprintf("dispatch: RPC error code %d is registered twice", spec.code))
		}
		types[spec.problemType] = true
		codes[spec.code] = true
	}
	return specs
}
