package planning

import (
	"errors"
	"fmt"
)

// Outcome is the Planning-owned semantic reason a Goal-directed execution
// completed. It does not add states to the common Process lifecycle.
type Outcome string

const (
	// OutcomeAchieved means the latest observed WorldState satisfies the Goal.
	OutcomeAchieved Outcome = "achieved"
	// OutcomeUnreachable means the initial complete planning search found no plan.
	OutcomeUnreachable Outcome = "unreachable"
	// OutcomeStuck means attempts or the Action limit were exhausted without
	// reaching the Goal.
	OutcomeStuck Outcome = "stuck"
)

// Valid reports whether outcome is one supported Planning completion reason.
func (outcome Outcome) Valid() bool {
	return outcome == OutcomeAchieved || outcome == OutcomeUnreachable || outcome == OutcomeStuck
}

// AttemptStatus records the observed result of one selected Action attempt.
type AttemptStatus string

const (
	// AttemptSucceeded means execution succeeded and reobservation established
	// every predicted effect.
	AttemptSucceeded AttemptStatus = "succeeded"
	// AttemptFailed means the dispatcher or child Process definitely failed.
	AttemptFailed AttemptStatus = "failed"
	// AttemptUnconfirmed means execution reported success but reobservation did
	// not establish every predicted effect.
	AttemptUnconfirmed AttemptStatus = "unconfirmed"
)

// Valid reports whether status is one supported Action attempt result.
func (status AttemptStatus) Valid() bool {
	return status == AttemptSucceeded || status == AttemptFailed || status == AttemptUnconfirmed
}

// Attempt is one final, portable Action-attempt fact. Diagnostic is empty only
// for a succeeded attempt.
type Attempt struct {
	// ActionName is the exact Action identity selected for this attempt.
	ActionName string `json:"action_name"`
	// Status is the observed semantic outcome of this attempt.
	Status AttemptStatus `json:"status"`
	// Diagnostic explains failed or unconfirmed attempts and is empty on success.
	Diagnostic string `json:"diagnostic,omitempty"`
}

// Validate verifies the Action identity, status, and bounded diagnostic.
func (attempt Attempt) Validate() error {
	if !validName(attempt.ActionName) || !attempt.Status.Valid() {
		return errors.New("planning: invalid Action attempt identity or status")
	}
	if attempt.Status == AttemptSucceeded {
		if attempt.Diagnostic != "" {
			return errors.New("planning: succeeded Action attempt has a diagnostic")
		}
		return nil
	}
	if !validDiagnostic(attempt.Diagnostic) {
		return errors.New("planning: failed or unconfirmed Action attempt requires a bounded diagnostic")
	}
	return nil
}

// Output is the final semantic Planning result. WorldState is the last complete
// observation, Attempts preserve selection order, and PlanningPasses counts
// calls to Planner. No field is derived from Event or Delta history.
type Output struct {
	// Outcome is the Planning-owned semantic completion reason.
	Outcome Outcome `json:"outcome"`
	// WorldState is the final complete observation.
	WorldState WorldState `json:"world_state"`
	// Attempts preserves Action-attempt order.
	Attempts []Attempt `json:"attempts"`
	// PlanningPasses counts calls to Planner.
	PlanningPasses uint32 `json:"planning_passes"`
}

// Validate verifies that Output is internally consistent with its outcome.
func (output Output) Validate() error {
	if !output.Outcome.Valid() || !output.WorldState.Valid() {
		return errors.New("planning: invalid output outcome or WorldState")
	}
	for index, attempt := range output.Attempts {
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("planning: output attempt %d: %w", index, err)
		}
	}
	switch output.Outcome {
	case OutcomeAchieved:
		return nil
	case OutcomeUnreachable:
		if len(output.Attempts) != 0 || output.PlanningPasses != 1 {
			return errors.New("planning: unreachable output requires one initial planning pass and no attempts")
		}
	case OutcomeStuck:
		if len(output.Attempts) == 0 {
			return errors.New("planning: stuck output requires at least one attempt")
		}
	}
	return nil
}
