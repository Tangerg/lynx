package operation

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/scope/app/runtime/protocol"
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

func (c *CapabilityGapError) Error() string {
	names := make([]string, 0, len(c.Requirements))
	for _, requirement := range c.Requirements {
		names = append(names, string(requirement.Type)+"."+requirement.Name)
	}
	return fmt.Sprintf("%s: requires %s", protocol.ErrCapabilityNotNeg, strings.Join(names, ", "))
}

func (c *CapabilityGapError) Is(target error) bool { return target == protocol.ErrCapabilityNotNeg }

func (c *CapabilityGapError) Enrich(problem *protocol.ProblemData) {
	problem.RequiredCapabilities = slices.Clone(c.Requirements)
}

// ActiveRunConflictError carries the non-terminal root that refused admission.
type ActiveRunConflictError struct {
	ActiveRun protocol.ActiveRunRef
}

func (a *ActiveRunConflictError) Error() string {
	return fmt.Sprintf("%s: run %s is %s", protocol.ErrSessionHasActiveRun, a.ActiveRun.RunID, a.ActiveRun.Status)
}

func (a *ActiveRunConflictError) Is(target error) bool {
	return target == protocol.ErrSessionHasActiveRun
}

func (a *ActiveRunConflictError) Enrich(problem *protocol.ProblemData) {
	activeRun := a.ActiveRun
	problem.ActiveRun = &activeRun
}
