package protocol

import (
	"errors"
	"strings"
	"testing"
)

func TestRuntimeEventWireConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event RuntimeEvent
		field string
	}{{
		name: "valid files change",
		event: RuntimeEvent{
			Type: RuntimeFilesChanged, Sequence: 1, Paths: []string{"README.md"},
		},
	}, {
		name: "sequence starts at one",
		event: RuntimeEvent{
			Type: RuntimeSkillsChanged, Sequence: 0,
		},
		field: "sequence",
	}, {
		name: "files change needs a concrete path",
		event: RuntimeEvent{
			Type: RuntimeFilesChanged, Sequence: 1, Paths: []string{},
		},
		field: "paths",
	}, {
		name: "resync names its recovery scope",
		event: RuntimeEvent{
			Type: RuntimeResync, Sequence: 1,
		},
		field: "topics",
	}, {
		name: "resync scope is not empty",
		event: RuntimeEvent{
			Type: RuntimeResync, Sequence: 1, Topics: []RuntimeTopic{},
		},
		field: "topics",
	}, {
		name: "optional scope is nonempty when present",
		event: RuntimeEvent{
			Type: RuntimeSessionsChanged, Sequence: 1, SessionIDs: []string{},
		},
		field: "sessionIds",
	}, {
		name: "variant fields stay closed",
		event: RuntimeEvent{
			Type: RuntimeSkillsChanged, Sequence: 1, RunIDs: []string{"run_1"},
		},
		field: "runIds",
	}}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.event.ValidateWire()
			if test.field == "" {
				if err != nil {
					t.Fatalf("ValidateWire rejected a valid event: %v", err)
				}
				return
			}
			assertConstraintField(t, err, "RuntimeEvent", test.field)
		})
	}
}

func TestOutputCollectionWireConstraints(t *testing.T) {
	t.Parallel()

	pending := PendingInterruptSet{Interrupts: []Interrupt{}}
	assertConstraintField(t, pending.ValidateWire(), "PendingInterruptSet", "interrupts")

	capability := ProblemData{
		Type:                 ErrCapabilityNotNeg.Error(),
		RequiredCapabilities: []CapabilityRequirement{},
	}
	assertConstraintField(t, capability.ValidateWire(), "ProblemData", "requiredCapabilities")

	duplicate := CapabilityRequirement{Type: RequirementFeature, Name: "subagents"}
	capability.RequiredCapabilities = []CapabilityRequirement{duplicate, duplicate}
	assertConstraintField(t, capability.ValidateWire(), "ProblemData", "requiredCapabilities")
}

func TestPublishedLimitWireConstraints(t *testing.T) {
	t.Parallel()

	replay := RunReplayLimits{Scope: ReplayScopeProcessRootSegment}
	assertConstraintField(t, replay.ValidateWire(), "RunReplayLimits", "maxEvents")

	subscription := SubscriptionLimits{}
	err := subscription.ValidateWire()
	assertConstraintField(t, err, "SubscriptionLimits", "maxTopics")
	assertConstraintField(t, err, "SubscriptionLimits", "maxWatches")
}

func assertConstraintField(t *testing.T, err error, shape, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ValidateWire accepted invalid %s.%s", shape, field)
	}
	constraint, ok := errors.AsType[*ConstraintError](err)
	if !ok {
		t.Fatalf("ValidateWire error = %T %v, want *ConstraintError", err, err)
	}
	for _, violation := range constraint.Fields {
		if violation.Field == field {
			if !strings.Contains(err.Error(), shape+"."+field) {
				t.Fatalf("error = %q, want shape-qualified path %s.%s", err, shape, field)
			}
			return
		}
	}
	t.Fatalf("violations = %+v, want field %q", constraint.Fields, field)
}
