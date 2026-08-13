package runtimeembedded

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

type runtimeProblemError struct {
	data  protocol.ProblemData
	cause error
}

func requireRuntimeContractViolation(t testing.TB, err error) {
	t.Helper()
	if !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("error = %v, want ErrIncompatibleRuntime", err)
	}
}

func (err runtimeProblemError) Error() string                 { return err.data.Type }
func (err runtimeProblemError) Unwrap() error                 { return err.cause }
func (err runtimeProblemError) Problem() protocol.ProblemData { return err.data }

func TestClassifyErrorPreservesIdentityAndProjectsRecoveryMetadata(t *testing.T) {
	t.Parallel()

	source := runtimeProblemError{
		cause: protocol.ErrCapabilityNotNeg,
		data: protocol.ProblemData{
			Type: protocol.ErrCapabilityNotNeg.Error(), Detail: "declare the missing topic",
			DocURL: "https://docs.example/discovery",
			RequiredCapabilities: []protocol.CapabilityRequirement{{
				Type: protocol.RequirementRuntimeTopic, Name: "files.changed",
			}},
		},
	}
	err := classifyError(source)
	if !errors.Is(err, agent.ErrIncompatibleRuntime) || !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("classified identities = %v", err)
	}
	var wire protocol.ProblemError
	if !errors.As(err, &wire) || wire.Problem().Type != protocol.ErrCapabilityNotNeg.Error() {
		t.Fatalf("runtime problem was not preserved: %T %v", err, err)
	}
	problem, ok := failure.FromError(err)
	if !ok || problem == nil || len(problem.RequiredCapabilities) != 1 ||
		problem.RequiredCapabilities[0].Kind != failure.RequirementRuntimeTopic ||
		problem.RequiredCapabilities[0].Name != "files.changed" {
		t.Fatalf("projected failure = (%+v, %v)", problem, ok)
	}
	for _, want := range []string{"declare the missing topic", "docs.example", "runtimeTopic:files.changed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("projected error omitted %q: %s", want, err)
		}
	}
	problem.Detail = "mutated"
	again, _ := failure.FromError(err)
	if again.Detail != "declare the missing topic" {
		t.Fatal("projected error leaked mutable failure state")
	}
}

func TestClassifyErrorExposesCommandReplaySemantics(t *testing.T) {
	for _, test := range []struct {
		source error
		want   error
	}{
		{source: protocol.ErrIdempotencyInProgress, want: agent.ErrCommandInProgress},
		{source: protocol.ErrIdempotencyConflict, want: agent.ErrCommandConflict},
		{source: protocol.ErrIdempotencyStoreMismatch, want: agent.ErrCommandStoreMismatch},
	} {
		err := classifyError(runtimeProblemError{cause: test.source, data: protocol.ProblemData{Type: test.source.Error()}})
		if !errors.Is(err, test.want) || !errors.Is(err, test.source) {
			t.Fatalf("classified %v = %v, want %v", test.source, err, test.want)
		}
	}
}

func TestClassifyErrorProjectsUnmappedRuntimeProblem(t *testing.T) {
	t.Parallel()

	source := runtimeProblemError{
		cause: errors.New("provider cause"),
		data: protocol.ProblemData{
			Type: protocol.ProblemRateLimited, RetryAfterSeconds: 4,
		},
	}
	err := classifyError(source)
	problem, ok := failure.FromError(err)
	if !ok || problem.Type != protocol.ProblemRateLimited || problem.RetryAfterSeconds != 4 || !errors.Is(err, source.cause) {
		t.Fatalf("unmapped runtime problem = (%+v, %v), err=%v", problem, ok, err)
	}
}

func TestClassifyErrorRejectsMalformedRuntimeProblem(t *testing.T) {
	t.Parallel()
	source := runtimeProblemError{
		cause: errors.New("malformed provider cause"),
		data:  protocol.ProblemData{Type: protocol.ProblemRateLimited, RetryAfterSeconds: -1},
	}
	err := classifyError(source)
	requireRuntimeContractViolation(t, err)
	if problem, ok := failure.FromError(err); ok {
		t.Fatalf("malformed runtime failure crossed the adapter boundary: %+v", problem)
	}
	if !errors.Is(err, source.cause) {
		t.Fatalf("contract violation lost the runtime cause: %v", err)
	}
}
