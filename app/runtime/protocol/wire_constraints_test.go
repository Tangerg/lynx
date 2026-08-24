package protocol

import (
	"errors"
	"slices"
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

func TestAgentMemoryTargetIsUnambiguous(t *testing.T) {
	t.Parallel()

	workspace := &WorkspaceRef{Path: "/repo"}
	for _, request := range []AgentMemoryListRequest{
		{Scope: AgentMemoryScopeProject, Workspace: workspace},
		{Scope: AgentMemoryScopeUser},
	} {
		if err := request.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected valid target %+v: %v", request, err)
		}
	}

	assertConstraintField(
		t,
		(AgentMemoryListRequest{Scope: AgentMemoryScopeProject}).ValidateWire(),
		"AgentMemoryListRequest",
		"workspace",
	)
	assertConstraintField(
		t,
		(AgentMemoryListRequest{Scope: AgentMemoryScopeUser, Workspace: workspace}).ValidateWire(),
		"AgentMemoryListRequest",
		"workspace",
	)
	assertConstraintField(
		t,
		(AgentMemoryAddRequest{Scope: AgentMemoryScopeUser, Workspace: workspace, Content: "fact"}).ValidateWire(),
		"AgentMemoryAddRequest",
		"workspace",
	)
}

func TestAgentMemoryContentWireConstraintUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	const maximum = 4096
	content := strings.Repeat("界", maximum)
	if err := (AgentMemoryAddRequest{
		Scope: AgentMemoryScopeUser, Content: content,
	}).ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected the memory content boundary: %v", err)
	}

	content += "界"
	assertConstraintField(
		t,
		(AgentMemoryAddRequest{Scope: AgentMemoryScopeUser, Content: content}).ValidateWire(),
		"AgentMemoryAddRequest",
		"content",
	)
	assertConstraintField(
		t,
		(AgentMemoryUpdateRequest{ID: "mem_1", Content: &content}).ValidateWire(),
		"AgentMemoryUpdateRequest",
		"content",
	)
	blank := ""
	assertConstraintField(
		t,
		(AgentMemoryUpdateRequest{ID: "mem_1", Content: &blank}).ValidateWire(),
		"AgentMemoryUpdateRequest",
		"content",
	)
	assertConstraintField(
		t,
		(AgentMemoryItem{
			ID: "mem_1", Scope: AgentMemoryScopeUser, Content: content,
			Origin: AgentMemoryOriginUser, Status: AgentMemoryStatusActive,
		}).ValidateWire(),
		"AgentMemoryItem",
		"content",
	)
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

func TestQuestionWireConstraints(t *testing.T) {
	t.Parallel()

	valid := Question{Fields: []QuestionField{{
		Type:    QuestionFieldChoice,
		Prompt:  "Choose",
		Header:  "选择一下",
		Options: []QuestionOption{{Label: "A"}, {Label: "B"}},
	}}}
	if err := ValidateWireTree(valid); err != nil {
		t.Fatalf("ValidateWireTree rejected a valid question: %v", err)
	}

	oneOption := valid
	oneOption.Fields = slices.Clone(valid.Fields)
	oneOption.Fields[0].Options = []QuestionOption{{Label: "A"}}
	assertConstraintField(t, ValidateWireTree(oneOption), "Question", "fields[0].options")

	longHeader := valid
	longHeader.Fields = slices.Clone(valid.Fields)
	longHeader.Fields[0].Header = "一二三四五六七八九十一二三"
	assertConstraintField(t, ValidateWireTree(longHeader), "Question", "fields[0].header")
}

func TestMCPSecretMapChangesRejectEmptyReplacement(t *testing.T) {
	t.Parallel()

	headers := MCPHeadersChange{Type: MCPSecretSet, Value: map[string]string{}}
	assertConstraintField(t, headers.ValidateWire(), "MCPHeadersChange", "value")

	environment := MCPEnvironmentChange{Type: MCPSecretSet, Value: map[string]string{}}
	assertConstraintField(t, environment.ValidateWire(), "MCPEnvironmentChange", "value")

	headers.Value = map[string]string{"X-API-Key": "secret"}
	if err := headers.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected a non-empty headers replacement: %v", err)
	}
}

func TestUpdateScheduleWorkspaceModesAreUnambiguous(t *testing.T) {
	t.Parallel()

	valid := []UpdateScheduleRequest{
		{ID: "sch_1", ExpectedRevision: 1},
		{ID: "sch_1", ExpectedRevision: 1, Workspace: &WorkspaceRef{Path: "/workspace"}},
		{ID: "sch_1", ExpectedRevision: 1, WorkspaceMode: ScheduleWorkspaceDefault},
	}
	for _, request := range valid {
		if err := ValidateWireTree(request); err != nil {
			t.Errorf("ValidateWireTree rejected legal schedule workspace patch %+v: %v", request, err)
		}
	}

	assertConstraintField(t, ValidateWireTree(UpdateScheduleRequest{
		ID:               "sch_1",
		ExpectedRevision: 1,
		Workspace:        &WorkspaceRef{Path: "/workspace"},
		WorkspaceMode:    ScheduleWorkspaceDefault,
	}), "UpdateScheduleRequest", "workspace")
	assertConstraintField(t, (UpdateScheduleRequest{
		ID:               "sch_1",
		ExpectedRevision: 1,
		WorkspaceMode:    "unknown",
	}).ValidateWire(), "UpdateScheduleRequest", "workspaceMode")
}

func TestMCPWireUnionsAcceptEveryLegalBranch(t *testing.T) {
	t.Parallel()

	valid := []WireValidator{
		MCPConnection{Type: MCPTransportStreamableHTTP, URL: "https://example.com/mcp"},
		MCPConnection{Type: MCPTransportStdio, Command: "mcp-server"},
		MCPConnectionInput{Type: MCPTransportStreamableHTTP, URL: "https://example.com/mcp"},
		MCPConnectionInput{Type: MCPTransportStdio, Command: "mcp-server"},
		MCPAuthorizationChange{Type: MCPSecretSet, Value: "Bearer secret"},
		MCPAuthorizationChange{Type: MCPSecretClear},
		MCPHeadersChange{Type: MCPSecretClear},
		MCPEnvironmentChange{Type: MCPSecretClear},
	}
	for _, value := range valid {
		if err := value.ValidateWire(); err != nil {
			t.Errorf("ValidateWire rejected legal %T branch: %v", value, err)
		}
	}

	stdioCandidate := MCPServerCandidate{
		Name: "filesystem", Enabled: false,
		Connection: MCPConnectionInput{Type: MCPTransportStdio, Command: "mcp-server"},
	}
	if err := ValidateWireTree(stdioCandidate); err != nil {
		t.Fatalf("ValidateWireTree rejected a legal stdio candidate: %v", err)
	}

	assertConstraintField(t,
		(MCPConnectionInput{Type: MCPTransportStdio}).ValidateWire(),
		"MCPConnectionInput", "command",
	)
	assertConstraintField(t,
		(MCPConnectionInput{Type: MCPTransportStreamableHTTP}).ValidateWire(),
		"MCPConnectionInput", "url",
	)
	assertConstraintField(t,
		(MCPAuthorizationChange{Type: MCPSecretSet}).ValidateWire(),
		"MCPAuthorizationChange", "value",
	)
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

func TestCancelRunReasonWireConstraintUsesUnicodeCharacters(t *testing.T) {
	t.Parallel()

	const maximum = 1024
	valid := CancelRunRequest{RunID: "run_1", Reason: strings.Repeat("界", maximum)}
	if err := valid.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected the cancellation reason boundary: %v", err)
	}

	valid.Reason += "界"
	assertConstraintField(t, valid.ValidateWire(), "CancelRunRequest", "reason")
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
				Question: &Question{Fields: []QuestionField{{
					Prompt: "Continue?", Type: QuestionFieldText,
				}}},
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

func TestItemTimingVocabularyIsVariantExclusive(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	message := Item{
		ID: "item_message", RunID: "run_1", Status: ItemStatusCompleted,
		Type: ItemTypeUserMessage, CreatedAt: at,
	}
	if err := message.ValidateWire(); err != nil {
		t.Fatalf("message timing: %v", err)
	}
	message.StartedAt = at
	assertConstraintField(t, message.ValidateWire(), "Item", "startedAt")

	toolCall := Item{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning,
		Type: ItemTypeToolCall, StartedAt: at,
	}
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("tool-call timing: %v", err)
	}
	toolCall.CreatedAt = at
	assertConstraintField(t, toolCall.ValidateWire(), "Item", "createdAt")

	finishedAt := at.Add(time.Minute)
	toolCall = Item{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusIncomplete,
		Type: ItemTypeToolCall, StartedAt: at, FinishedAt: finishedAt,
	}
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("terminal tool-call with unknown execution duration: %v", err)
	}
	durationMillis := int64(500)
	toolCall.DurationMillis = &durationMillis
	if err := toolCall.ValidateWire(); err != nil {
		t.Fatalf("terminal tool-call with exact execution duration: %v", err)
	}

	artifactToolCall := ArtifactItem{
		ID: "item_tool", RunID: "run_1", Status: ItemStatusRunning,
		Type: ItemTypeToolCall, StartedAt: at,
	}
	if err := artifactToolCall.ValidateWire(); err != nil {
		t.Fatalf("artifact tool-call timing: %v", err)
	}
	artifactToolCall.CreatedAt = at
	assertConstraintField(t, artifactToolCall.ValidateWire(), "ArtifactItem", "createdAt")
}

func TestPublishedLimitWireConstraints(t *testing.T) {
	t.Parallel()

	replay := RunReplayLimits{Scope: ReplayScopeRuntimeInstanceRootSegment}
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

func TestModelSelectionWireConstraintsRequireAnExactPair(t *testing.T) {
	t.Parallel()

	start := StartRunRequest{
		SessionID: "ses_1",
		Input:     []ContentBlock{{Type: ContentBlockText, Text: "go"}},
		Provider:  "provider",
	}
	assertConstraintField(t, start.ValidateWire(), "StartRunRequest", "model")

	provider := "provider"
	update := UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Provider: &provider,
	}
	assertConstraintField(t, update.ValidateWire(), "UpdateSessionRequest", "model")

	model := "model"
	update = UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Model: &model,
	}
	assertConstraintField(t, update.ValidateWire(), "UpdateSessionRequest", "provider")
}

func TestRequestBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "PageQuery", field: "limit", value: PageQuery{Limit: -1}},
		{shape: "GetDiffRequest", field: "limit", value: GetDiffRequest{Limit: -1}},
		{shape: "GetFileHeadRequest", field: "lines", value: GetFileHeadRequest{Path: "README.md", Lines: -1}},
		{shape: "GrepRequest", field: "limit", value: GrepRequest{Query: "needle", Limit: -1}},
		{shape: "ReadFileRequest", field: "startLine", value: ReadFileRequest{Path: "README.md", StartLine: -1}},
		{shape: "ReadFileRequest", field: "endLine", value: ReadFileRequest{Path: "README.md", EndLine: -1}},
		{shape: "ReadFileRequest", field: "maxBytes", value: ReadFileRequest{Path: "README.md", MaxBytes: -1}},
		{shape: "UsageSummaryRequest", field: "sinceDays", value: UsageSummaryRequest{SinceDays: -1}},
	} {
		t.Run(test.shape+"."+test.field, func(t *testing.T) {
			t.Parallel()
			assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
		})
	}
}

func TestGenerationAndGoalBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	temperature := 2.1
	topP := 1.1
	zeroTokens := int64(0)
	for _, test := range []struct {
		field string
		value GenerationParams
	}{
		{field: "temperature", value: GenerationParams{Temperature: &temperature}},
		{field: "topP", value: GenerationParams{TopP: &topP}},
		{field: "maxTokens", value: GenerationParams{MaxTokens: &zeroTokens}},
		{field: "stop", value: GenerationParams{Stop: []string{}}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), "GenerationParams", test.field)
	}

	assertConstraintField(
		t,
		(GoalBudget{MaxCostUSD: -0.01}).ValidateWire(),
		"GoalBudget",
		"maxCostUsd",
	)
}

func TestSessionArtifactBoundsAreWireConstraints(t *testing.T) {
	t.Parallel()

	artifact := SessionArtifact{Version: SessionArtifactVersion - 1}
	assertConstraintField(t, artifact.ValidateWire(), "SessionArtifact", "version")
	artifact.Version = SessionArtifactVersion + 1
	assertConstraintField(t, artifact.ValidateWire(), "SessionArtifact", "version")

	cost := -0.01
	for _, test := range []struct {
		shape string
		field string
		value WireValidator
	}{
		{shape: "ArtifactRun", field: "messageMark", value: ArtifactRun{MessageMark: -1}},
		{shape: "ArtifactRunMetrics", field: "steps", value: ArtifactRunMetrics{Steps: -1}},
		{shape: "ArtifactRunMetrics", field: "activeDurationMillis", value: ArtifactRunMetrics{ActiveDurationMillis: -1}},
		{shape: "ArtifactUsage", field: "inputTokens", value: ArtifactUsage{InputTokens: -1}},
		{shape: "ArtifactUsage", field: "costUsd", value: ArtifactUsage{CostUSD: &cost}},
		{shape: "ArtifactModelUsage", field: "reasoningTokens", value: ArtifactModelUsage{ReasoningTokens: -1}},
		{shape: "ArtifactItem", field: "droppedMessages", value: ArtifactItem{DroppedMessages: -1}},
		{shape: "ArtifactProblem", field: "retryAfterSeconds", value: ArtifactProblem{RetryAfterSeconds: -1}},
	} {
		assertConstraintField(t, test.value.ValidateWire(), test.shape, test.field)
	}
}

func TestOptionalMCPUpdateConstraintsPreserveAndValidatePresentValues(t *testing.T) {
	t.Parallel()

	request := UpdateMCPServerRequest{Server: "files"}
	if err := request.ValidateWire(); err != nil {
		t.Fatalf("ValidateWire rejected an omission-only patch: %v", err)
	}

	negative := -1
	request.TimeoutSeconds = &negative
	assertConstraintField(t, request.ValidateWire(), "UpdateMCPServerRequest", "timeoutSeconds")

	repeated := []string{"read", "read"}
	request.TimeoutSeconds = nil
	request.DisabledTools = &repeated
	assertConstraintField(t, request.ValidateWire(), "UpdateMCPServerRequest", "disabledTools")
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
