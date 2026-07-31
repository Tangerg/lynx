package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// Capability negotiation: what a request is entitled to, resolved once.
//
// runs.start freezes the answer onto the new Run as its protocol profile;
// runs.resume and runs.subscribe hand the same answer to the application, which
// refuses a caller that cannot cover what the Run already publishes. Both readings
// come from this one function, so "what you may ask for" and "what you must be
// able to follow" can never become two vocabularies that disagree.

// negotiateCapabilities resolves what the caller declared against what this build
// advertises (§8.1).
//
// A declared capability this build cannot honor is a refusal, never a silent drop.
// Dropping it is the failure mode the contract names explicitly: a client that
// asked for subagents and received an ordinary run would fold a stream believing
// child events could appear in it, and a client whose interrupt type was quietly
// discarded would be handed a wait it never said it could answer.
//
// Absent capabilities are the Minimal Profile, not an error — §8.3 makes "send a
// message, watch the reply, reload the history" a complete client.
func (s *Server) negotiateCapabilities(ctx context.Context) (execution.RunProtocolProfile, error) {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok {
		return execution.RunProtocolProfile{}, nil
	}

	advertised := s.Capabilities().Features

	var profile execution.RunProtocolProfile
	for key, preference := range caps.Features {
		if !preference.Enabled {
			// Declining a feature is always honorable, including one this build has
			// never heard of.
			continue
		}
		published, known := protocol.LookupFeature(key)
		if !known || !advertised[key].Enabled {
			return execution.RunProtocolProfile{}, protocol.NewCapabilityGap(protocol.CapabilityRequirement{
				Type: protocol.RequirementFeature, Name: key,
			})
		}
		// Only a feature that changes what the Run PUBLISHES belongs on the Run: the
		// profile exists so a later subscriber can be told what it must understand,
		// and a feature invisible in the stream demands nothing of it.
		if published.RequiredByRunProtocol {
			switch key {
			case protocol.FeatureSubagents:
				profile.ChildRuns = true
			default:
				return execution.RunProtocolProfile{}, fmt.Errorf(
					"server: required Run protocol feature %q has no application policy mapping",
					key,
				)
			}
		}
	}

	for _, declared := range caps.InterruptTypes {
		kind, backed := interruptKindFromWire(declared)
		if backed {
			profile.InterruptKinds = append(profile.InterruptKinds, kind)
			continue
		}
		if declared == protocol.InterruptToolResult {
			// A client tool's wait is raised for and answered by the CLIENT, so the
			// runtime can only produce it with features.clientTools. Both gaps are named:
			// the type the caller asked for and the feature that would make it possible,
			// because fixing only one of them changes nothing.
			return execution.RunProtocolProfile{}, protocol.NewCapabilityGap(
				protocol.CapabilityRequirement{Type: protocol.RequirementInterruptType, Name: string(declared)},
				protocol.CapabilityRequirement{Type: protocol.RequirementFeature, Name: protocol.FeatureClientTools},
			)
		}
		return execution.RunProtocolProfile{}, fmt.Errorf("%w: unknown interruptTypes value %q", protocol.ErrInvalidParams, declared)
	}
	return profile.Normalized(), nil
}

// missingFeatureRequirements is the server-side entry point for a gate whose
// trigger depends on durable state and therefore cannot live in MethodMeta.When.
// The actual server/client decision is shared with the static dispatcher gate.
func (s *Server) missingFeatureRequirements(
	ctx context.Context,
	required ...string,
) []protocol.CapabilityRequirement {
	var client *protocol.ClientCapabilities
	if declared, ok := protocol.ClientCapabilitiesFrom(ctx); ok {
		client = declared
	}
	return protocol.MissingFeatureRequirements(
		s.Capabilities().Features, client, required...,
	)
}

func (s *Server) requireFeature(ctx context.Context, feature string) error {
	missing := s.missingFeatureRequirements(ctx, feature)
	if len(missing) == 0 {
		return nil
	}
	return protocol.NewCapabilityGap(missing...)
}

func (s *Server) requestCanUseFeature(ctx context.Context, feature string) bool {
	return len(s.missingFeatureRequirements(ctx, feature)) == 0
}

// profileGap turns a Run's uncovered profile into the requirements a caller would
// have to declare. It is the same list in both directions: what the Run publishes,
// spoken as what the caller is missing.
//
// Every gap at once, because a caller told about one at a time cannot get itself into
// a state where the call succeeds.
func profileGap(gap execution.RunProtocolProfile) *protocol.CapabilityGap {
	requirements := make([]protocol.CapabilityRequirement, 0,
		1+len(gap.InterruptKinds))
	if gap.ChildRuns {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature, Name: protocol.FeatureSubagents,
		})
	}
	for _, kind := range gap.InterruptKinds {
		requirements = append(requirements, protocol.CapabilityRequirement{
			Type: protocol.RequirementInterruptType, Name: string(presentInterruptType(kind)),
		})
	}
	return protocol.NewCapabilityGap(requirements...)
}

// interruptKindFromWire maps a declared interrupt type onto the durable kind the
// runtime raises. It reports false for a type no kind backs — which is not the
// same as an unknown value, and the caller distinguishes them.
func interruptKindFromWire(kind protocol.InterruptType) (execution.InterruptKind, bool) {
	switch kind {
	case protocol.InterruptApproval:
		return execution.ApprovalInterrupt, true
	case protocol.InterruptQuestion:
		return execution.QuestionInterrupt, true
	default:
		return 0, false
	}
}

// presentInterruptType is the same mapping read outward, for a profile or an
// interrupt on its way to the wire.
func presentInterruptType(kind execution.InterruptKind) protocol.InterruptType {
	switch kind {
	case execution.ApprovalInterrupt:
		return protocol.InterruptApproval
	case execution.QuestionInterrupt:
		return protocol.InterruptQuestion
	default:
		panic("server: unknown interrupt kind")
	}
}
