// Package failure defines structured failure details shared by the CLI's
// bounded contexts.
//
// A problem is a CLI-owned value object, not the runtime wire type. Adapters
// translate into it once so agent runs, MCP management, model configuration,
// terminal presentation, and machine renderers cannot silently disagree about
// recovery metadata.
package failure

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type problemCarrier interface {
	Failure() *Problem
}

// RequirementKind names the capability registry that contains a missing
// capability.
type RequirementKind string

const (
	RequirementFeature       RequirementKind = "feature"
	RequirementInterruptType RequirementKind = "interruptType"
	RequirementRuntimeTopic  RequirementKind = "runtimeTopic"
)

func (kind RequirementKind) valid() bool {
	switch kind {
	case RequirementFeature, RequirementInterruptType, RequirementRuntimeTopic:
		return true
	default:
		return false
	}
}

// CapabilityRequirement identifies one capability a caller must negotiate.
type CapabilityRequirement struct {
	Kind RequirementKind `json:"type"`
	Name string          `json:"name"`
}

// ActiveRun identifies the run preventing a conflicting session operation.
// It is an observation, not a command target; callers re-fetch before acting.
type ActiveRun struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

// FieldError identifies one invalid input field.
type FieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// Problem is a structured failure with the information a client needs to
// explain the failure and offer an appropriate recovery action.
type Problem struct {
	Type                 string                  `json:"type"`
	Detail               string                  `json:"detail,omitempty"`
	DocURL               string                  `json:"docUrl,omitempty"`
	RetryAfterSeconds    int                     `json:"retryAfterSeconds,omitempty"`
	RequiredCapabilities []CapabilityRequirement `json:"requiredCapabilities,omitempty"`
	ActiveRun            *ActiveRun              `json:"activeRun,omitempty"`
	Errors               []FieldError            `json:"errors,omitempty"`
}

// FromError returns the CLI-owned structured failure carried anywhere in an
// error chain. The returned problem is independently owned.
func FromError(err error) (*Problem, bool) {
	if err == nil {
		return nil, false
	}
	var carrier problemCarrier
	if !errors.As(err, &carrier) {
		return nil, false
	}
	problem := carrier.Failure()
	return problem, problem != nil
}

// Validate checks the portable structure without copying the runtime's
// provider-extensible problem-type registry into the CLI core.
func (problem Problem) Validate() error {
	var problems []error
	if strings.TrimSpace(problem.Type) == "" {
		problems = append(problems, errors.New("type is empty"))
	}
	if problem.RetryAfterSeconds < 0 {
		problems = append(problems, errors.New("retry delay is negative"))
	}
	seen := make(map[CapabilityRequirement]struct{}, len(problem.RequiredCapabilities))
	for index, requirement := range problem.RequiredCapabilities {
		if !requirement.Kind.valid() {
			problems = append(problems, fmt.Errorf("required capability %d has invalid kind %q", index+1, requirement.Kind))
		}
		if strings.TrimSpace(requirement.Name) == "" {
			problems = append(problems, fmt.Errorf("required capability %d has an empty name", index+1))
		}
		if _, duplicate := seen[requirement]; duplicate {
			problems = append(problems, fmt.Errorf("required capability %d duplicates %s:%s", index+1, requirement.Kind, requirement.Name))
		}
		seen[requirement] = struct{}{}
	}
	if problem.ActiveRun != nil {
		if strings.TrimSpace(problem.ActiveRun.RunID) == "" {
			problems = append(problems, errors.New("active run id is empty"))
		}
		switch problem.ActiveRun.Status {
		case "running", "waiting", "finished":
		default:
			problems = append(problems, fmt.Errorf("active run status %q is invalid", problem.ActiveRun.Status))
		}
	}
	for index, field := range problem.Errors {
		if strings.TrimSpace(field.Field) == "" {
			problems = append(problems, fmt.Errorf("field error %d has an empty field", index+1))
		}
		if strings.TrimSpace(field.Detail) == "" {
			problems = append(problems, fmt.Errorf("field error %d has an empty detail", index+1))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("problem: %w", err)
	}
	return nil
}

// Clone returns an independently owned problem. It is safe on a nil receiver.
func (problem *Problem) Clone() *Problem {
	if problem == nil {
		return nil
	}
	cloned := *problem
	cloned.RequiredCapabilities = slices.Clone(problem.RequiredCapabilities)
	cloned.Errors = slices.Clone(problem.Errors)
	if problem.ActiveRun != nil {
		cloned.ActiveRun = new(*problem.ActiveRun)
	}
	return &cloned
}

// Equal reports whether two optional problems carry the same information.
func (problem *Problem) Equal(other *Problem) bool {
	if problem == nil || other == nil {
		return problem == other
	}
	if problem.Type != other.Type || problem.Detail != other.Detail || problem.DocURL != other.DocURL ||
		problem.RetryAfterSeconds != other.RetryAfterSeconds || !slices.Equal(problem.RequiredCapabilities, other.RequiredCapabilities) ||
		!slices.Equal(problem.Errors, other.Errors) || (problem.ActiveRun == nil) != (other.ActiveRun == nil) {
		return false
	}
	return problem.ActiveRun == nil || *problem.ActiveRun == *other.ActiveRun
}

// Message returns the most useful concise explanation for status lines.
func (problem Problem) Message(fallback string) string {
	if detail := strings.TrimSpace(problem.Detail); detail != "" {
		return detail
	}
	if problemType := strings.TrimSpace(problem.Type); problemType != "" {
		return problemType
	}
	return fallback
}

// String returns a complete, single-line human projection. Machine consumers
// marshal Problem directly and therefore retain the same information without
// parsing this text.
func (problem Problem) String() string {
	parts := []string{problem.Type}
	if detail := strings.TrimSpace(problem.Detail); detail != "" {
		parts[0] += ": " + detail
	}
	if problem.RetryAfterSeconds > 0 {
		parts = append(parts, fmt.Sprintf("retry after %ds", problem.RetryAfterSeconds))
	}
	if problem.DocURL != "" {
		parts = append(parts, "docs "+problem.DocURL)
	}
	if len(problem.RequiredCapabilities) > 0 {
		required := make([]string, 0, len(problem.RequiredCapabilities))
		for _, capability := range problem.RequiredCapabilities {
			required = append(required, string(capability.Kind)+":"+capability.Name)
		}
		parts = append(parts, "requires "+strings.Join(required, ", "))
	}
	if problem.ActiveRun != nil {
		parts = append(parts, fmt.Sprintf("active run %s (%s)", problem.ActiveRun.RunID, problem.ActiveRun.Status))
	}
	if len(problem.Errors) > 0 {
		fields := make([]string, 0, len(problem.Errors))
		for _, field := range problem.Errors {
			fields = append(fields, field.Field+": "+field.Detail)
		}
		parts = append(parts, "fields "+strings.Join(fields, ", "))
	}
	return strings.Join(parts, " · ")
}
