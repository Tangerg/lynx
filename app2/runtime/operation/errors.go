package operation

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// ProblemDetailed is implemented by an internal service error that contributes
// structured fields to a client-visible protocol problem.
type ProblemDetailed interface {
	error
	Enrich(*protocol.ProblemData)
}

// CapabilityGapError carries every missing capability of one operation.
type CapabilityGapError struct {
	Requirements []protocol.CapabilityRequirement
}

// NewCapabilityGapError sorts and deduplicates capability requirements.
func NewCapabilityGapError(requirements ...protocol.CapabilityRequirement) *CapabilityGapError {
	slices.SortStableFunc(requirements, func(a, b protocol.CapabilityRequirement) int {
		if a.Type != b.Type {
			return cmp.Compare(requirementOrder(a.Type), requirementOrder(b.Type))
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return &CapabilityGapError{Requirements: slices.Compact(requirements)}
}

func requirementOrder(kind protocol.CapabilityRequirementType) int {
	return slices.Index([]protocol.CapabilityRequirementType{
		protocol.RequirementFeature,
		protocol.RequirementInterruptType,
		protocol.RequirementRuntimeTopic,
	}, kind)
}

func (e *CapabilityGapError) Error() string {
	names := make([]string, 0, len(e.Requirements))
	for _, requirement := range e.Requirements {
		names = append(names, string(requirement.Type)+"."+requirement.Name)
	}
	return fmt.Sprintf("%s: requires %s", protocol.ErrCapabilityNotNeg, strings.Join(names, ", "))
}

func (e *CapabilityGapError) Is(target error) bool { return target == protocol.ErrCapabilityNotNeg }

func (e *CapabilityGapError) Enrich(problem *protocol.ProblemData) {
	problem.RequiredCapabilities = slices.Clone(e.Requirements)
}

// ActiveRunConflictError carries the non-terminal root that refused admission.
type ActiveRunConflictError struct {
	ActiveRun protocol.ActiveRunRef
}

func (e *ActiveRunConflictError) Error() string {
	return fmt.Sprintf("%s: run %s is %s", protocol.ErrSessionHasActiveRun, e.ActiveRun.RunID, e.ActiveRun.Status)
}

func (e *ActiveRunConflictError) Is(target error) bool {
	return target == protocol.ErrSessionHasActiveRun
}

func (e *ActiveRunConflictError) Enrich(problem *protocol.ProblemData) {
	activeRun := e.ActiveRun
	problem.ActiveRun = &activeRun
}
