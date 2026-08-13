package operation

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type problemSpec struct {
	sentinel          error
	retryAfterSeconds int
	methodDeclarable  bool
	recovery          protocol.RecoveryAction
}

var problemSpecs = mustProblemSpecs([]problemSpec{
	frameworkProblem(protocol.ErrInvalidRequest, protocol.RecoveryStop),
	frameworkProblem(protocol.ErrInternalError, protocol.RecoveryStop),
	frameworkProblem(protocol.ErrMethodNotFound, protocol.RecoveryStop),
	frameworkProblem(protocol.ErrInvalidParams, protocol.RecoveryStop),
	declaredProblem(protocol.ErrSessionNotFound, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrRunNotFound, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrItemNotFound, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrMCPServerNotFound, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrMCPServerAlreadyExists, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrMCPServerDisabled, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrMCPAuthorizationAttemptNotFound, protocol.RecoveryStop),
	declaredProblem(protocol.ErrRunNotRoot, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrSessionBusy, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrRevisionConflict, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrInterruptNotOpen, protocol.RecoveryColdRecover),
	declaredProblem(protocol.ErrReplayUnavailable, protocol.RecoveryColdRecover),
	declaredProblem(protocol.ErrRunWaiting, protocol.RecoveryColdRecover),
	declaredProblem(protocol.ErrRunFinished, protocol.RecoveryColdRecover),
	declaredProblem(protocol.ErrStaleSegment, protocol.RecoveryRefetch),
	declaredProblem(protocol.ErrReplayCursorInvalid, protocol.RecoveryResubscribe),
	declaredProblem(protocol.ErrSessionHasActiveRun, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrCapabilityNotNeg, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrWorkspaceUnavailable, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrCheckpointUnavailable, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrUnsupportedMime, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrPathOutsideRoot, protocol.RecoveryPromptUser),
	declaredProblem(protocol.ErrVcsUnavailable, protocol.RecoveryPromptUser),
	frameworkProblem(protocol.ErrInvalidProtocolVersion, protocol.RecoveryPromptUser),
	frameworkProblem(protocol.ErrIdempotencyConflict, protocol.RecoveryPromptUser),
	frameworkProblem(protocol.ErrIdempotencyStoreMismatch, protocol.RecoveryColdRecover),
	declaredProblem(protocol.ErrProviderError, protocol.RecoveryPromptUser),
	retryingProblem(protocol.ErrIdempotencyInProgress, 1),
})

func frameworkProblem(sentinel error, recovery protocol.RecoveryAction) problemSpec {
	return problemSpec{sentinel: sentinel, recovery: recovery}
}

func declaredProblem(sentinel error, recovery protocol.RecoveryAction) problemSpec {
	return problemSpec{sentinel: sentinel, methodDeclarable: true, recovery: recovery}
}

func retryingProblem(sentinel error, retryAfterSeconds int) problemSpec {
	return problemSpec{
		sentinel:          sentinel,
		retryAfterSeconds: retryAfterSeconds,
		recovery:          protocol.RecoveryWaitRetryAfter,
	}
}

// Failure is a safely projected operation failure. Bindings expose Problem while
// errors.Is still reaches the stable protocol sentinel through Cause.
type Failure struct {
	cause error
	data  protocol.ProblemData
}

var _ protocol.ProblemError = (*Failure)(nil)

func (f *Failure) Error() string {
	if f == nil {
		return ""
	}
	if f.data.Detail != "" {
		return f.data.Detail
	}
	return f.data.Type
}

func (f *Failure) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.cause
}

// Problem returns a defensive copy of the client-visible problem.
func (f *Failure) Problem() protocol.ProblemData {
	if f == nil {
		return protocol.ProblemData{}
	}
	return cloneProblemData(f.data)
}

// NewFailure constructs a safe failure for a registered stable sentinel.
func NewFailure(sentinel error, detail string) *Failure {
	spec, ok := problemSpecForError(sentinel)
	if !ok {
		return internalFailure()
	}
	return newFailure(spec, sentinel, detail)
}

// ProjectError maps an implementation error onto the binding-neutral problem
// vocabulary without exposing arbitrary internal details for unknown failures.
func ProjectError(err error) *Failure {
	if err == nil {
		return nil
	}
	if failure, ok := errors.AsType[*Failure](err); ok {
		return &Failure{cause: failure.cause, data: cloneProblemData(failure.data)}
	}
	for _, spec := range problemSpecs {
		if !errors.Is(err, spec.sentinel) {
			continue
		}
		failure := newFailure(spec, err, err.Error())
		if detailed, ok := errors.AsType[ProblemDetailed](err); ok {
			detailed.Enrich(&failure.data)
		}
		if protocol.ValidateWireTree(failure.data) != nil {
			return internalFailure()
		}
		return failure
	}
	return internalFailure()
}

// InvalidParameters projects strict wire-constraint failures with their field
// locations intact.
func InvalidParameters(err error) *Failure {
	failure := NewFailure(protocol.ErrInvalidParams, err.Error())
	if constraint, ok := errors.AsType[*protocol.ConstraintError](err); ok {
		failure.data.Errors = slices.Clone(constraint.Fields)
	}
	if protocol.ValidateWireTree(failure.data) != nil {
		return internalFailure()
	}
	return failure
}

func newFailure(spec problemSpec, cause error, detail string) *Failure {
	return &Failure{
		cause: cause,
		data: protocol.ProblemData{
			Type:              spec.sentinel.Error(),
			Detail:            detail,
			RetryAfterSeconds: spec.retryAfterSeconds,
		},
	}
}

func internalFailure() *Failure {
	spec, _ := problemSpecForType(protocol.ProblemInternalError)
	return newFailure(spec, protocol.ErrInternalError, "the runtime could not complete the request")
}

func failureFromData(data protocol.ProblemData) *Failure {
	spec, ok := problemSpecForType(data.Type)
	if !ok || protocol.ValidateWireTree(data) != nil {
		return internalFailure()
	}
	return &Failure{cause: spec.sentinel, data: cloneProblemData(data)}
}

func cloneProblemData(data protocol.ProblemData) protocol.ProblemData {
	data.RequiredCapabilities = slices.Clone(data.RequiredCapabilities)
	data.Errors = slices.Clone(data.Errors)
	if data.ActiveRun != nil {
		active := *data.ActiveRun
		data.ActiveRun = &active
	}
	return data
}

// RecoveryFor returns the safe default recovery action for a problem type.
func RecoveryFor(problemType string) (protocol.RecoveryAction, bool) {
	spec, ok := problemSpecForType(problemType)
	return spec.recovery, ok
}

// IsMethodProblemType reports whether method metadata may declare the type.
func IsMethodProblemType(problemType string) bool {
	spec, ok := problemSpecForType(problemType)
	return ok && spec.methodDeclarable
}

// ProblemTypes returns every registered first-party problem type.
func ProblemTypes() []string {
	out := make([]string, 0, len(problemSpecs))
	for _, spec := range problemSpecs {
		out = append(out, spec.sentinel.Error())
	}
	return out
}

func RetryAfterFor(problemType string) int {
	spec, _ := problemSpecForType(problemType)
	return spec.retryAfterSeconds
}

func problemSpecForError(err error) (problemSpec, bool) {
	for _, spec := range problemSpecs {
		if errors.Is(err, spec.sentinel) {
			return spec, true
		}
	}
	return problemSpec{}, false
}

func problemSpecForType(problemType string) (problemSpec, bool) {
	for _, spec := range problemSpecs {
		if spec.sentinel.Error() == problemType {
			return spec, true
		}
	}
	return problemSpec{}, false
}

func mustProblemSpecs(specs []problemSpec) []problemSpec {
	seen := make(map[string]bool, len(specs))
	for index, spec := range specs {
		switch {
		case spec.sentinel == nil:
			panic(fmt.Sprintf("operation: problem spec %d has no sentinel", index))
		case seen[spec.sentinel.Error()]:
			panic(fmt.Sprintf("operation: problem type %q is registered twice", spec.sentinel))
		case !spec.recovery.Valid():
			panic(fmt.Sprintf("operation: problem type %q has invalid recovery %q", spec.sentinel, spec.recovery))
		case spec.recovery == protocol.RecoveryWaitRetryAfter && spec.retryAfterSeconds <= 0:
			panic(fmt.Sprintf("operation: problem type %q waits without retryAfterSeconds", spec.sentinel))
		case spec.recovery != protocol.RecoveryWaitRetryAfter && spec.retryAfterSeconds != 0:
			panic(fmt.Sprintf("operation: problem type %q publishes retryAfterSeconds for recovery %q", spec.sentinel, spec.recovery))
		}
		seen[spec.sentinel.Error()] = true
	}
	return specs
}
