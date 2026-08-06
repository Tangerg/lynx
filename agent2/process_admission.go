package agent2

import (
	"errors"
	"fmt"
)

// ErrProcessAdmissionRejected reports that the configured ProcessAdmitter did
// not approve a prospective root or child Process. The Process is never
// published or started.
var ErrProcessAdmissionRejected = errors.New("agent: process admission rejected")

// ProcessAdmission is the immutable Framework-owned information supplied to a
// ProcessAdmitter immediately before one root or child Process starts. It does
// not expose Input, Execution, Dispatcher, product identity, or Host state.
type ProcessAdmission struct {
	relation      ProcessRelation
	deploymentRef DeploymentRef
	descriptor    Descriptor
	budget        Budget
	capabilities  CapabilitySet
}

// Relation returns the prospective Process identity and tree location.
func (admission ProcessAdmission) Relation() ProcessRelation { return admission.relation }

// DeploymentRef returns the exact prospective Deployment identity.
func (admission ProcessAdmission) DeploymentRef() DeploymentRef { return admission.deploymentRef }

// Descriptor returns the prospective Definition's static contract.
func (admission ProcessAdmission) Descriptor() Descriptor { return admission.descriptor }

// Budget returns the prospective Process's fixed non-renewable allocation.
func (admission ProcessAdmission) Budget() Budget { return admission.budget }

// Capabilities returns the prospective Process's immutable authority set.
func (admission ProcessAdmission) Capabilities() CapabilitySet { return admission.capabilities }

// Valid reports whether the admission contains one coherent prospective
// Process. Only the Engine constructs admission values.
func (admission ProcessAdmission) Valid() bool {
	return admission.relation.Valid() && admission.deploymentRef.Valid() &&
		admission.descriptor.Valid() && admission.budget.Valid() &&
		admission.capabilities.Valid() &&
		admission.deploymentRef.Name() == admission.descriptor.Name() &&
		admission.deploymentRef.Version() == admission.descriptor.Version() &&
		admission.deploymentRef.ContractDigest() == admission.descriptor.Digest()
}

// ProcessAdmitter decides whether one prospective root or child Process may
// start. Implementations are decision-only: they must not create a Process,
// mutate the admission, allocate resources, or treat a call as a durable
// charge. A prepared Step may replay the same child admission after recovery.
//
// Implementations must be synchronous, bounded, safe for concurrent calls when
// shared, and must not perform external I/O or re-enter a Process. Returning an
// error rejects only this prospective Process. Budget allocation, capability
// attenuation, and tree limits remain Engine invariants and cannot be changed
// by an admitter. Restore does not repeat admission for a Process whose admitted
// state was captured.
type ProcessAdmitter interface {
	Admit(admission ProcessAdmission) error
}

// ProcessAdmitterFunc adapts a function to ProcessAdmitter.
type ProcessAdmitterFunc func(admission ProcessAdmission) error

// Admit invokes admitter.
func (admitter ProcessAdmitterFunc) Admit(admission ProcessAdmission) error {
	return admitter(admission)
}

func newProcessAdmission(
	relation ProcessRelation,
	deployment Deployment,
	budget Budget,
	capabilities CapabilitySet,
) ProcessAdmission {
	return ProcessAdmission{
		relation: relation, deploymentRef: deployment.DeploymentRef(),
		descriptor: deployment.Descriptor(), budget: budget,
		capabilities: capabilities,
	}
}

func requestProcessAdmission(
	admitter ProcessAdmitter,
	admission ProcessAdmission,
) (err error) {
	if admitter == nil {
		return nil
	}
	if !admission.Valid() {
		return fmt.Errorf("%w: invalid admission", ErrProcessAdmissionRejected)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf(
				"%w: admitter panicked: %v",
				ErrProcessAdmissionRejected, recovered,
			)
		}
	}()
	if err := admitter.Admit(admission); err != nil {
		return fmt.Errorf("%w: %w", ErrProcessAdmissionRejected, err)
	}
	return nil
}
