package agent2

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	startedAt     time.Time
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

// StartedAt returns the prospective Process start time. If the accepted
// admission concludes with a started outcome, this exact UTC value becomes the
// Process lifecycle start.
func (admission ProcessAdmission) StartedAt() time.Time { return admission.startedAt }

// Valid reports whether the admission contains one coherent prospective
// Process. Only the Engine constructs admission values.
func (admission ProcessAdmission) Valid() bool {
	return admission.relation.Valid() && admission.deploymentRef.Valid() &&
		admission.descriptor.Valid() && admission.budget.Valid() &&
		admission.capabilities.Valid() && !admission.startedAt.IsZero() &&
		admission.startedAt.Location() == time.UTC &&
		admission.deploymentRef.Name() == admission.descriptor.Name() &&
		admission.deploymentRef.Version() == admission.descriptor.Version() &&
		admission.deploymentRef.ContractDigest() == admission.descriptor.Digest()
}

// ProcessAdmitter decides whether one prospective root or child Process may
// initialize. Implementations may coordinate caller-owned external admission work,
// but must not create a Process, mutate the admission, or allocate Framework
// resources. A prepared Step may replay the same child admission with the same
// prospective Process identity after recovery.
//
// Implementations must respect ctx, return in bounded time, be safe for
// concurrent calls when shared, and must not re-enter the Engine or a Process.
// Framework identity is stable, but persistence, transactionality, charging,
// and business idempotency remain implementation responsibilities. Returning
// an error rejects only this prospective Process. Budget allocation, capability
// attenuation, and tree limits remain Engine invariants and cannot be changed
// by an admitter. Every accepted admission concludes with exactly one
// ProcessStartOutcome when an acknowledger is configured. Restore repeats
// neither admission nor its outcome for a captured Process.
type ProcessAdmitter interface {
	Admit(ctx context.Context, admission ProcessAdmission) error
}

// ProcessAdmitterFunc adapts a function to ProcessAdmitter.
type ProcessAdmitterFunc func(ctx context.Context, admission ProcessAdmission) error

// Admit invokes admitter.
func (admitter ProcessAdmitterFunc) Admit(ctx context.Context, admission ProcessAdmission) error {
	return admitter(ctx, admission)
}

func newProcessAdmission(
	relation ProcessRelation,
	deployment Deployment,
	budget Budget,
	capabilities CapabilitySet,
	startedAt time.Time,
) ProcessAdmission {
	return ProcessAdmission{
		relation: relation, deploymentRef: deployment.DeploymentRef(),
		descriptor: deployment.Descriptor(), budget: budget,
		capabilities: capabilities, startedAt: startedAt,
	}
}

func requestProcessAdmission(
	ctx context.Context,
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
	if err := admitter.Admit(contextOrBackground(ctx), admission); err != nil {
		return fmt.Errorf("%w: %w", ErrProcessAdmissionRejected, err)
	}
	return nil
}
