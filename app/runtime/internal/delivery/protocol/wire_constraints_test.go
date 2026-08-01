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

func TestProblemDataWireUnion(t *testing.T) {
	t.Parallel()

	validCapability := []CapabilityRequirement{{Type: RequirementFeature, Name: "subagents"}}
	tests := []struct {
		name    string
		problem ProblemData
		field   string
	}{
		{name: "ordinary first party problem", problem: ProblemData{Type: ProblemRunLost}},
		{
			name:    "inline status carries no server authored copy",
			problem: ProblemData{Type: ProblemMCPDialFailed, Detail: "connection failed"},
			field:   "detail",
		},
		{
			name: "capability problem carries its gaps",
			problem: ProblemData{
				Type: ErrCapabilityNotNeg.Error(), RequiredCapabilities: validCapability,
			},
		},
		{
			name:    "capability problem requires gaps",
			problem: ProblemData{Type: ErrCapabilityNotNeg.Error()},
			field:   "requiredCapabilities",
		},
		{
			name:    "structured fields belong to their variant",
			problem: ProblemData{Type: ProblemRunLost, RequiredCapabilities: validCapability},
			field:   "requiredCapabilities",
		},
		{
			name: "active run belongs only to the conflict",
			problem: ProblemData{
				Type:      ProblemRunLost,
				ActiveRun: &ActiveRunRef{RunID: "run_1", Status: RunStatusRunning},
			},
			field: "activeRun",
		},
		{
			name:    "idempotency progress requires a delay",
			problem: ProblemData{Type: ErrIdempotencyInProgress.Error()},
			field:   "retryAfterSeconds",
		},
		{
			name:    "retry delay is positive",
			problem: ProblemData{Type: ProblemTimeout, RetryAfterSeconds: -1},
			field:   "retryAfterSeconds",
		},
		{
			name: "plugin problem uses its namespace",
			problem: ProblemData{
				Type: "plugin:acme/model_timeout", Detail: "try another region",
				RetryAfterSeconds: 2,
			},
		},
		{
			name: "plugin problem cannot borrow first party fields",
			problem: ProblemData{
				Type: "plugin:acme/model_timeout", RequiredCapabilities: validCapability,
			},
			field: "requiredCapabilities",
		},
		{
			name:    "unnamespaced extension is rejected",
			problem: ProblemData{Type: "model_timeout"},
			field:   "type",
		},
		{
			name:    "malformed plugin namespace is rejected",
			problem: ProblemData{Type: "plugin:Acme/model_timeout"},
			field:   "type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateWireTree(test.problem)
			if test.field == "" {
				if err != nil {
					t.Fatalf("ValidateWireTree rejected a valid problem: %v", err)
				}
				return
			}
			assertConstraintField(t, err, "ProblemData", test.field)
		})
	}
}

func TestProblemDataStructuredLeavesAreValidated(t *testing.T) {
	t.Parallel()

	activeRun := ProblemData{
		Type:      ErrSessionHasActiveRun.Error(),
		ActiveRun: &ActiveRunRef{Status: RunStatus("teleported")},
	}
	err := ValidateWireTree(activeRun)
	assertConstraintField(t, err, "ProblemData", "activeRun.runId")
	assertConstraintField(t, err, "ProblemData", "activeRun.status")

	invalidFields := ProblemData{
		Type:   ErrInvalidParams.Error(),
		Errors: []FieldError{{}},
	}
	err = ValidateWireTree(invalidFields)
	assertConstraintField(t, err, "ProblemData", "errors[0].field")
	assertConstraintField(t, err, "ProblemData", "errors[0].detail")
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

	start := StartRunRequest{
		Input:          []ContentBlock{{Type: ContentBlockText, Text: "go"}},
		MaxTotalTokens: -1,
	}
	assertConstraintField(t, start.ValidateWire(), "StartRunRequest", "maxTotalTokens")

	run := RunLimits{MaxSteps: -1}
	assertConstraintField(t, run.ValidateWire(), "RunLimits", "maxSteps")

	artifact := ArtifactRunLimits{MaxBudgetUSD: -0.01}
	assertConstraintField(t, artifact.ValidateWire(), "ArtifactRunLimits", "maxBudgetUsd")

	if err := (RunLimits{}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected uncapped RunLimits: %v", err)
	}
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
