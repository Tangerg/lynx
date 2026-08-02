package dispatch

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// rpcErrorSpecs maps each protocol sentinel to its wire behavior (§8.2 / §9.3).
// The ordered registry makes errors.Is precedence deterministic. The symbolic
// ProblemData.type is the sentinel's Error string; clients branch on it, not on the
// numeric code.
//
// Every entry declares a recovery action. It replaces the transient/permanent flag
// the contract rejected: "retryable" made a client guess what to do about a failure,
// while the action says it. Which METHODS may return a type is not declared here —
// each registration already lists the errors it returns, and a second list is a second
// answer.
type rpcErrorSpec struct {
	sentinel          error
	code              int
	retryAfterSeconds int
	// methodDeclarable says a method registration may name this problem as a
	// business refusal. Envelope, metadata and idempotency boundary failures are
	// still published, but cannot be misattributed to an individual handler.
	methodDeclarable bool
	// recovery is the safe default next move. It never overrides a method's
	// idempotency policy and never authorizes replaying the user's intent.
	recovery protocol.RecoveryAction
}

var rpcErrorSpecs = mustRPCErrorSpecs([]rpcErrorSpec{
	{sentinel: protocol.ErrInvalidRequest, code: protocol.CodeInvalidRequest, recovery: protocol.RecoveryStop},
	{sentinel: protocol.ErrInternalError, code: protocol.CodeInternalError, recovery: protocol.RecoveryStop},
	// The request itself is wrong: nothing the client retries changes the answer, and
	// only a person can decide what it meant to ask.
	{sentinel: protocol.ErrMethodNotFound, code: protocol.CodeMethodNotFound, recovery: protocol.RecoveryStop},
	{sentinel: protocol.ErrInvalidParams, code: protocol.CodeInvalidParams, recovery: protocol.RecoveryStop},
	// The subject is gone, moved, or was never there. A client holding a stale id
	// reads again; what it finds may be that the thing is gone for good.
	{sentinel: protocol.ErrSessionNotFound, code: protocol.CodeSessionNotFound, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrRunNotFound, code: protocol.CodeRunNotFound, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrItemNotFound, code: protocol.CodeItemNotFound, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrMCPServerNotFound, code: protocol.CodeMCPServerNotFound, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrMCPServerAlreadyExists, code: protocol.CodeMCPServerExists, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrMCPServerDisabled, code: protocol.CodeMCPServerDisabled, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	// Authorization attempts are intentionally ephemeral. Once one expires, the
	// same id can never become readable again; repeating get would only repeat the
	// refusal, so the safe action is to stop polling it.
	{sentinel: protocol.ErrMCPAuthorizationAttemptNotFound, code: protocol.CodeMCPAuthorizationAttemptNotFound, recovery: protocol.RecoveryStop, methodDeclarable: true},
	{sentinel: protocol.ErrRunNotRoot, code: protocol.CodeRunNotRoot, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	// Something else holds the session or the working tree, or the revision moved.
	// Reading again is how the client learns whether it still does.
	{sentinel: protocol.ErrSessionBusy, code: protocol.CodeSessionBusy, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrRevisionConflict, code: protocol.CodeRevisionConflict, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	// The run moved on while the client was not looking: its STREAM is stale rather
	// than its ids, so it rebuilds from the durable reads.
	{sentinel: protocol.ErrInterruptNotOpen, code: protocol.CodeInterruptNotOpen, recovery: protocol.RecoveryColdRecover, methodDeclarable: true},
	{sentinel: protocol.ErrReplayUnavailable, code: protocol.CodeReplayUnavailable, recovery: protocol.RecoveryColdRecover, methodDeclarable: true},
	{sentinel: protocol.ErrRunWaiting, code: protocol.CodeRunWaiting, recovery: protocol.RecoveryColdRecover, methodDeclarable: true},
	{sentinel: protocol.ErrRunFinished, code: protocol.CodeRunFinished, recovery: protocol.RecoveryColdRecover, methodDeclarable: true},
	// The run is executing something else, or the client is holding a cursor that
	// never addressed this stream. Both are answered by reading the run again — and
	// for the cursor, by dropping it: reattaching with the same one would fail the
	// same way, so resubscribe means resubscribe WITHOUT it.
	{sentinel: protocol.ErrStaleSegment, code: protocol.CodeStaleSegment, recovery: protocol.RecoveryRefetch, methodDeclarable: true},
	{sentinel: protocol.ErrReplayCursorInvalid, code: protocol.CodeReplayCursorInvalid, recovery: protocol.RecoveryResubscribe, methodDeclarable: true},
	// Only a person can choose: which run continues, where to work, whether to
	// declare a capability, or what to do about a conflicting key.
	{sentinel: protocol.ErrSessionHasActiveRun, code: protocol.CodeSessionHasActiveRun, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrCapabilityNotNeg, code: protocol.CodeCapabilityNotNeg, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrWorkspaceUnavailable, code: protocol.CodeWorkspaceUnavailable, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrCheckpointUnavailable, code: protocol.CodeCheckpointUnavail, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrUnsupportedMime, code: protocol.CodeUnsupportedMime, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrPathOutsideRoot, code: protocol.CodePathOutsideRoot, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrVcsUnavailable, code: protocol.CodeVcsUnavailable, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	{sentinel: protocol.ErrInvalidProtocolVersion, code: protocol.CodeInvalidProtocolVersion, recovery: protocol.RecoveryPromptUser},
	{sentinel: protocol.ErrIdempotencyConflict, code: protocol.CodeIdempotencyConflict, recovery: protocol.RecoveryPromptUser},
	{sentinel: protocol.ErrProviderError, code: protocol.CodeProviderError, recovery: protocol.RecoveryPromptUser, methodDeclarable: true},
	// The same key is mid-flight: waiting is the whole remedy, and the hint says how
	// long. Retrying is safe here precisely because the key makes it the same call.
	{
		sentinel: protocol.ErrIdempotencyInProgress,
		code:     protocol.CodeIdempotencyInProgress, retryAfterSeconds: 1,
		recovery: protocol.RecoveryWaitRetryAfter,
	},
})

// RecoveryFor returns the declared default recovery action for one problem type, and
// false for a type no sentinel publishes. The generator reads it, so the published
// registry and the runtime's own table are one statement.
func RecoveryFor(problemType string) (protocol.RecoveryAction, bool) {
	spec, ok := specFor(problemType)
	return spec.recovery, ok
}

// IsMethodProblemType reports whether a method registration may declare this
// first-party problem. The fact lives beside the runtime's code and recovery
// behavior, so validation and generation cannot drift from a second name list.
func IsMethodProblemType(problemType string) bool {
	spec, ok := specFor(problemType)
	return ok && spec.methodDeclarable
}

// MethodProblemTypes lists every problem a method may declare, in deterministic
// registry order.
func MethodProblemTypes() []string {
	var out []string
	for _, spec := range rpcErrorSpecs {
		if spec.methodDeclarable {
			out = append(out, spec.sentinel.Error())
		}
	}
	return out
}

// ProblemCodes is the published business error surface: every problem type this
// dispatcher can send, with the code it is sent with.
//
// The generator reads it instead of keeping its own table. That table existed, and
// it was a verbatim copy — so a new error had to be remembered in two places, and
// the artifacts could publish a code the runtime does not send. A type reaches the
// published registry by having wire behavior here, not by being listed twice.
func ProblemCodes() map[string]int {
	out := make(map[string]int, len(rpcErrorSpecs))
	for _, spec := range rpcErrorSpecs {
		out[spec.sentinel.Error()] = spec.code
	}
	return out
}

// RetryAfterFor returns the declared backoff hint for one problem type. A hint says
// waiting may clear the condition; it never says a mutation is safe to repeat.
func RetryAfterFor(problemType string) int {
	spec, _ := specFor(problemType)
	return spec.retryAfterSeconds
}

func specFor(problemType string) (rpcErrorSpec, bool) {
	for _, spec := range rpcErrorSpecs {
		if spec.sentinel.Error() == problemType {
			return spec, true
		}
	}
	return rpcErrorSpec{}, false
}

func mustRPCErrorSpecs(specs []rpcErrorSpec) []rpcErrorSpec {
	types := make(map[string]bool, len(specs))
	codes := make(map[int]bool, len(specs))
	for index, spec := range specs {
		switch {
		case spec.sentinel == nil:
			panic(fmt.Sprintf("dispatch: RPC error spec %d has no sentinel", index))
		case spec.sentinel.Error() == "":
			panic(fmt.Sprintf("dispatch: RPC error spec %d has an empty problem type", index))
		case types[spec.sentinel.Error()]:
			panic(fmt.Sprintf(
				"dispatch: RPC problem type %q is registered twice",
				spec.sentinel,
			))
		case codes[spec.code]:
			panic(fmt.Sprintf(
				"dispatch: RPC error code %d is registered twice",
				spec.code,
			))
		case !spec.recovery.Valid():
			panic(fmt.Sprintf(
				"dispatch: RPC problem type %q has invalid recovery action %q",
				spec.sentinel,
				spec.recovery,
			))
		case spec.recovery == protocol.RecoveryWaitRetryAfter &&
			spec.retryAfterSeconds <= 0:
			panic(fmt.Sprintf(
				"dispatch: RPC problem type %q waits without a positive retryAfterSeconds",
				spec.sentinel,
			))
		case spec.recovery != protocol.RecoveryWaitRetryAfter &&
			spec.retryAfterSeconds != 0:
			panic(fmt.Sprintf(
				"dispatch: RPC problem type %q publishes retryAfterSeconds with recovery %q",
				spec.sentinel,
				spec.recovery,
			))
		}
		types[spec.sentinel.Error()] = true
		codes[spec.code] = true
	}
	return specs
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
	for _, spec := range rpcErrorSpecs {
		if errors.Is(err, spec.sentinel) {
			return problemFrame(spec, err)
		}
	}
	return problemError(protocol.ErrInternalError, "the runtime could not complete the request")
}

// problemError builds an Error carrying the registered sentinel's code and
// ProblemData type. Keeping both on the spec makes an impossible pair
// unrepresentable at call sites.
func problemError(sentinel error, detail string) *transport.Error {
	return problemErrorWithFields(sentinel, detail)
}

func problemErrorWithFields(sentinel error, detail string, fields ...protocol.FieldError) *transport.Error {
	if sentinel == nil {
		return invalidProblemResponse("the runtime could not encode an unregistered error response")
	}
	spec, ok := specFor(sentinel.Error())
	if !ok {
		return invalidProblemResponse("the runtime could not encode an unregistered error response")
	}
	return marshalProblem(spec, protocol.ProblemData{
		Type: sentinel.Error(), Detail: detail,
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
func problemFrame(spec rpcErrorSpec, err error) *transport.Error {
	problem := protocol.ProblemData{
		Type: spec.sentinel.Error(), Detail: err.Error(),
		RetryAfterSeconds: spec.retryAfterSeconds,
	}
	if detailed, ok := errors.AsType[protocol.ProblemDetailed](err); ok {
		detailed.Enrich(&problem)
	}
	return marshalProblem(spec, problem)
}

func marshalProblem(spec rpcErrorSpec, problem protocol.ProblemData) *transport.Error {
	if err := protocol.ValidateWireTree(problem); err != nil {
		return invalidProblemResponse("the runtime could not encode a valid error response")
	}
	encodedProblem, _ := json.Marshal(problem)
	return transport.NewError(spec.code, problem.Type, encodedProblem)
}

func invalidProblemResponse(detail string) *transport.Error {
	fallback := protocol.ProblemData{Type: protocol.ProblemInternalError, Detail: detail}
	encodedFallback, _ := json.Marshal(fallback)
	return transport.NewError(
		protocol.CodeInternalError,
		protocol.ProblemInternalError,
		encodedFallback,
	)
}

// invalidParams wraps a params-validation failure as invalid_params.
func invalidParams(reason string) *transport.Error {
	return problemError(protocol.ErrInvalidParams, reason)
}

// invalidRequestShape reports a decoded request that broke its own constraints.
// A [protocol.ConstraintError] names the offending params keys, so this fills
// ProblemData.errors (API.md §8.3) and the client flags each field instead of
// parsing one sentence. Anything else only has prose to offer.
func invalidRequestShape(err error) *transport.Error {
	if constraint, ok := errors.AsType[*protocol.ConstraintError](err); ok {
		return problemErrorWithFields(protocol.ErrInvalidParams, constraint.Error(), constraint.Fields...)
	}
	return invalidParams(err.Error())
}

// methodNotFound is the canonical envelope for an unknown method.
func methodNotFound(method string) *transport.Error {
	return problemError(protocol.ErrMethodNotFound,
		fmt.Sprintf("unknown method %q", method))
}

// badEnvelope is returned for malformed JSON-RPC envelopes (non-string
// id, wrong shape) at the dispatcher boundary.
func badEnvelope(detail string) *transport.Error {
	return problemError(protocol.ErrInvalidRequest, detail)
}
