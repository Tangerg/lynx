package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// sentinelSpecs maps each protocol sentinel to its wire behavior (§8.2 / §9.3).
// errors.Is checks identity, so iteration order is irrelevant. The symbolic
// ProblemData.type is the sentinel's Error string; clients branch on it, not on the
// numeric code.
//
// Every entry declares a recovery action. It replaces the transient/permanent flag
// the contract rejected: "retryable" made a client guess what to do about a failure,
// while the action says it. Which METHODS may return a type is not declared here —
// each registration already lists the errors it returns, and a second list is a second
// answer.
type rpcErrorSpec struct {
	code              int
	retryAfterSeconds int
	// recovery is the safe default next move. It never overrides a method's
	// idempotency policy and never authorizes replaying the user's intent.
	recovery protocol.RecoveryAction
}

var sentinelSpecs = map[error]rpcErrorSpec{
	// The request itself is wrong: nothing the client retries changes the answer, and
	// only a person can decide what it meant to ask.
	protocol.ErrMethodNotFound: {code: protocol.CodeMethodNotFound, recovery: protocol.RecoveryStop},
	protocol.ErrInvalidParams:  {code: protocol.CodeInvalidParams, recovery: protocol.RecoveryStop},
	// The subject is gone, moved, or was never there. A client holding a stale id
	// reads again; what it finds may be that the thing is gone for good.
	protocol.ErrSessionNotFound: {code: protocol.CodeSessionNotFound, recovery: protocol.RecoveryRefetch},
	protocol.ErrRunNotFound:     {code: protocol.CodeRunNotFound, recovery: protocol.RecoveryRefetch},
	protocol.ErrItemNotFound:    {code: protocol.CodeItemNotFound, recovery: protocol.RecoveryRefetch},
	protocol.ErrRunNotRoot:      {code: protocol.CodeRunNotRoot, recovery: protocol.RecoveryRefetch},
	protocol.ErrRunAlreadyDone:  {code: protocol.CodeRunAlreadyDone, recovery: protocol.RecoveryRefetch},
	// Something else holds the session or the working tree, or the revision moved.
	// Reading again is how the client learns whether it still does.
	protocol.ErrSessionBusy:      {code: protocol.CodeSessionBusy, recovery: protocol.RecoveryRefetch},
	protocol.ErrRevisionConflict: {code: protocol.CodeRevisionConflict, recovery: protocol.RecoveryRefetch},
	// The run moved on while the client was not looking: its STREAM is stale rather
	// than its ids, so it rebuilds from the durable reads.
	protocol.ErrInterruptNotOpen: {code: protocol.CodeInterruptNotOpen, recovery: protocol.RecoveryColdRecover},
	// Only a person can choose: which run continues, where to work, whether to
	// declare a capability, or what to do about a conflicting key.
	protocol.ErrSessionHasActiveRun:    {code: protocol.CodeSessionHasActiveRun, recovery: protocol.RecoveryPromptUser},
	protocol.ErrCapabilityNotNeg:       {code: protocol.CodeCapabilityNotNeg, recovery: protocol.RecoveryPromptUser},
	protocol.ErrCwdUnavailable:         {code: protocol.CodeCwdUnavailable, recovery: protocol.RecoveryPromptUser},
	protocol.ErrCheckpointUnavailable:  {code: protocol.CodeCheckpointUnavail, recovery: protocol.RecoveryPromptUser},
	protocol.ErrUnsupportedMime:        {code: protocol.CodeUnsupportedMime, recovery: protocol.RecoveryPromptUser},
	protocol.ErrPathOutsideRoot:        {code: protocol.CodePathOutsideRoot, recovery: protocol.RecoveryPromptUser},
	protocol.ErrVcsUnavailable:         {code: protocol.CodeVcsUnavailable, recovery: protocol.RecoveryPromptUser},
	protocol.ErrInvalidProtocolVersion: {code: protocol.CodeInvalidProtocolVersion, recovery: protocol.RecoveryPromptUser},
	protocol.ErrIdempotencyConflict:    {code: protocol.CodeIdempotencyConflict, recovery: protocol.RecoveryPromptUser},
	protocol.ErrProviderError:          {code: protocol.CodeProviderError, recovery: protocol.RecoveryPromptUser},
	// The same key is mid-flight: waiting is the whole remedy, and the hint says how
	// long. Retrying is safe here precisely because the key makes it the same call.
	protocol.ErrIdempotencyInProgress: {
		code: protocol.CodeIdempotencyInProgress, retryAfterSeconds: 1,
		recovery: protocol.RecoveryWaitRetryAfter,
	},
}

// RecoveryFor returns the declared default recovery action for one problem type, and
// false for a type no sentinel publishes. The generator reads it, so the published
// registry and the runtime's own table are one statement.
func RecoveryFor(problemType string) (protocol.RecoveryAction, bool) {
	spec, ok := specFor(problemType)
	return spec.recovery, ok && spec.recovery != ""
}

// RetryAfterFor returns the declared backoff hint for one problem type. A hint says
// waiting may clear the condition; it never says a mutation is safe to repeat.
func RetryAfterFor(problemType string) int {
	spec, _ := specFor(problemType)
	return spec.retryAfterSeconds
}

func specFor(problemType string) (rpcErrorSpec, bool) {
	for sentinel, spec := range sentinelSpecs {
		if sentinel.Error() == problemType {
			return spec, true
		}
	}
	return rpcErrorSpec{}, false
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
			return problemFrame(spec, sentinel.Error(), err)
		}
	}
	return problemError(protocol.CodeInternalError, protocol.ProblemInternalError, "the runtime could not complete the request")
}

// problemError builds an Error carrying a ProblemData{type, detail}.
// typ is the symbolic name (API.md §8.2); detail is the human string.
func problemError(code int, typ, detail string) *transport.Error {
	return problemErrorWithSpec(rpcErrorSpec{code: code}, typ, detail)
}

func problemErrorWithSpec(spec rpcErrorSpec, typ, detail string, fields ...protocol.FieldError) *transport.Error {
	return marshalProblem(spec, typ, protocol.ProblemData{
		Type: typ, Detail: detail,
		RetryAfterSeconds: spec.retryAfterSeconds,
		Errors:            fields,
	})
}

// problemFrame builds the frame for one error, letting the error fill the structured
// fields its problem type requires ([protocol.ProblemDetailed]).
//
// The alternative is a switch here that re-derives each type's payload from
// somewhere else — a second author for a fact the error already carried, and the one
// place it could go missing.
func problemFrame(spec rpcErrorSpec, typ string, err error) *transport.Error {
	data := protocol.ProblemData{
		Type: typ, Detail: err.Error(),
		RetryAfterSeconds: spec.retryAfterSeconds,
	}
	if detailed, ok := errors.AsType[protocol.ProblemDetailed](err); ok {
		detailed.Enrich(&data)
	}
	return marshalProblem(spec, typ, data)
}

func marshalProblem(spec rpcErrorSpec, typ string, problem protocol.ProblemData) *transport.Error {
	data, _ := json.Marshal(problem)
	return transport.NewError(spec.code, typ, data)
}

// invalidParams wraps a params-validation failure as invalid_params.
func invalidParams(reason string) *transport.Error {
	return problemError(protocol.CodeInvalidParams, "invalid_params", reason)
}

// invalidRequestShape reports a decoded request that broke its own constraints.
// A [protocol.ConstraintError] names the offending params keys, so this fills
// ProblemData.errors (API.md §8.3) and the client flags each field instead of
// parsing one sentence. Anything else only has prose to offer.
func invalidRequestShape(err error) *transport.Error {
	if constraint, ok := errors.AsType[*protocol.ConstraintError](err); ok {
		return problemErrorWithSpec(
			rpcErrorSpec{code: protocol.CodeInvalidParams},
			"invalid_params", constraint.Error(), constraint.Fields...,
		)
	}
	return invalidParams(err.Error())
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
