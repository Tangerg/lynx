package interaction

import (
	"fmt"
	"slices"
	"strings"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

const maxCompletionFeedbackBytes = 4096

// Artifact is one successful, schema-validated Delegate output. It is
// identified by the exact Delegate binding, never by a Go runtime type name or
// an application artifact store.
type Artifact struct {
	modelCall    uint32
	callID       string
	delegateName string
	output       agent.Output
}

// DelegateName returns the exact model-facing Delegate name.
func (artifact Artifact) DelegateName() string { return artifact.delegateName }

// Output returns the immutable, schema-validated child output.
func (artifact Artifact) Output() agent.Output { return artifact.output }

// DecodeArtifact strictly decodes an Artifact output into T. The output was
// already validated against the exact Delegate Descriptor before the Artifact
// was admitted to Interaction state; T is only an edge convenience.
func DecodeArtifact[T any](artifact Artifact) (T, error) {
	var zero T
	if !artifact.valid() {
		return zero, ErrInvalidArtifact
	}
	value, err := agent.DecodeOutput[T](artifact.output)
	if err != nil {
		return zero, fmt.Errorf("%w: decode: %w", ErrInvalidArtifact, err)
	}
	return value, nil
}

func (artifact Artifact) valid() bool {
	return artifact.modelCall > 0 && artifact.callID != "" &&
		artifact.delegateName != "" && artifact.output.Valid()
}

// Artifacts is an immutable, ordered snapshot of successful Delegate outputs.
// All returns defensive copies, so a validator cannot mutate Execution state.
type Artifacts struct {
	values []Artifact
}

// Len returns the number of successful Delegate outputs accumulated so far.
func (artifacts Artifacts) Len() int { return len(artifacts.values) }

// All returns Artifacts in original model ToolCall order across model calls.
func (artifacts Artifacts) All() []Artifact { return slices.Clone(artifacts.values) }

func newArtifacts(records []artifactRecord) Artifacts {
	values := make([]Artifact, len(records))
	for index, record := range records {
		values[index] = Artifact{
			modelCall: record.ModelCall, callID: record.CallID,
			delegateName: record.DelegateName, output: record.Output,
		}
	}
	return Artifacts{values: values}
}

// CompletionCandidate is the immutable model context and semantic output
// proposed by an Interaction together with all successful Delegate Artifacts
// available at that boundary.
type CompletionCandidate struct {
	workingContext *chat.Request
	output         Output
	artifacts      Artifacts
}

// WorkingContext returns an independently owned copy of the model context
// preceding this candidate. It is Interaction state, not Host conversation or
// transcript history, and it does not yet contain the candidate Output.
func (candidate CompletionCandidate) WorkingContext() *chat.Request {
	return candidate.workingContext.Clone()
}

// Output returns an independently owned candidate Output.
func (candidate CompletionCandidate) Output() Output { return cloneOutput(candidate.output) }

// Artifacts returns the immutable Delegate output snapshot.
func (candidate CompletionCandidate) Artifacts() Artifacts { return candidate.artifacts }

// CompletionDecision is the explicit result of a CompletionValidator.
// Accepted=true requires empty Feedback. Accepted=false requires concise,
// non-empty Feedback that will be appended as a user message before the next
// model call.
type CompletionDecision struct {
	Accepted bool
	Feedback string
}

// Valid reports whether the decision is internally consistent and bounded.
func (decision CompletionDecision) Valid() bool {
	if decision.Accepted {
		return decision.Feedback == ""
	}
	return decision.Feedback != "" && strings.TrimSpace(decision.Feedback) == decision.Feedback &&
		len(decision.Feedback) <= maxCompletionFeedbackBytes
}

// CompletionValidator decides whether a model or direct-Tool candidate is a
// valid semantic completion. It must be bounded, deterministic and
// side-effect-free: no I/O, clock, randomness, shared mutation or goroutines.
// A rejected candidate must return actionable Feedback; MaxModelCalls remains
// the hard bound on retry rounds. Evaluation requiring external work belongs
// in a managed child Process, not this callback.
type CompletionValidator func(CompletionCandidate) (CompletionDecision, error)

func cloneOutput(output Output) Output {
	cloned := output
	cloned.ModelResponse = cloneResponse(output.ModelResponse)
	cloned.DirectToolResults = slices.Clone(output.DirectToolResults)
	return cloned
}
