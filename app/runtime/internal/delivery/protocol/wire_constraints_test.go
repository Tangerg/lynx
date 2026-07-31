package protocol

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestRunOpeningResponseWireConstraints(t *testing.T) {
	t.Parallel()

	start := StartRunResponse{RunID: "run_1", SegmentID: "seg_1"}
	assertConstraintField(t, start.ValidateWire(), "StartRunResponse", "userItemId")
	start.UserItemID = "item_1"
	if err := start.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a complete start response: %v", err)
	}

	resume := ResumeRunResponse{RunID: "run_1", SegmentID: "seg_2"}
	if err := resume.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a response-only resume: %v", err)
	}
	empty := ""
	resume.UserItemID = &empty
	assertConstraintField(t, resume.ValidateWire(), "ResumeRunResponse", "userItemId")
}

func TestRunProtocolProfileWireConstraints(t *testing.T) {
	t.Parallel()

	valid := RunProtocolProfile{
		RequiredFeatures: []RunProtocolFeature{RunProtocolFeatureSubagents},
		InterruptTypes:   []InterruptType{InterruptApproval, InterruptQuestion},
	}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a valid profile: %v", err)
	}

	repeatedFeature := valid
	repeatedFeature.RequiredFeatures = append(repeatedFeature.RequiredFeatures, RunProtocolFeatureSubagents)
	assertConstraintField(t, repeatedFeature.ValidateWire(), "RunProtocolProfile", "requiredFeatures")

	repeatedInterrupt := valid
	repeatedInterrupt.InterruptTypes = append(repeatedInterrupt.InterruptTypes, InterruptApproval)
	assertConstraintField(t, repeatedInterrupt.ValidateWire(), "RunProtocolProfile", "interruptTypes")

	unknown := valid
	unknown.RequiredFeatures = []RunProtocolFeature{"telepathy"}
	assertConstraintField(t, unknown.ValidateWire(), "RunProtocolProfile", "requiredFeatures[0]")
}

func TestValidateWireTreeComposesNestedConstraints(t *testing.T) {
	t.Parallel()

	pending := PendingInterruptSet{
		RootRunID: "run_root",
		SessionID: "ses_1",
		CreatedAt: time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC),
		Interrupts: []Interrupt{{
			ItemID: "item_question",
			Type:   InterruptQuestion,
			Payload: &InterruptPayload{
				Question: &Question{Prompt: "Continue?"},
			},
		}},
	}
	assertConstraintField(t, pending.Interrupts[0].ValidateWire(), "Interrupt", "runId")
	assertConstraintField(
		t,
		ValidateWireTree(pending),
		"PendingInterruptSet",
		"interrupts[0].runId",
	)
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
