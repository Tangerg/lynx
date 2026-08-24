package dispatch

import (
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// The value constraints of wire shapes.
//
// They were hand-written request checks or output assumptions, which meant the
// schema and runtime could disagree with no mechanical signal. Declared here, one
// statement generates the Go validator, JSON Schema and TypeScript validator.
//
// The kinds are the ones a JSON type does not already imply: a string that may not
// be empty, a number that may not be zero, an array that may not be sent empty, an
// array that may not repeat. Closed-enum membership is derived from the enum's own
// declared value set, not restated here.

func registerValueConstraints(s *Shapes) {
	registerCollectionValues(s)
	registerSessionValues(s)
	registerArtifactValues(s)
	registerRunValues(s)
	registerPlanValues(s)
	registerWorkspaceValues(s)
	registerUsageValues(s)
	registerSkillValues(s)
	registerHookValues(s)
	registerApprovalValues(s)
	registerMCPValues(s)
	registerProviderValues(s)
	registerToolValues(s)
	registerKnowledgeValues(s)
	registerAgentMemoryValues(s)
	registerScheduleValues(s)
	registerGoalValues(s)
	registerRuntimeValues(s)
}

func registerCollectionValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.PageQuery](),
		Constraints: []FieldConstraint{{Field: "limit", Kind: ConstraintNonNegative}},
	})
}

// nonEmpty builds the common spec: these fields are ids or text that must be there.
func nonEmpty[Request any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonEmpty})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Request](), Constraints: constraints})
}

// nonNegative builds the common accounting spec. Token counts, costs, step
// counts and durations are facts already consumed; a negative value is not an
// alternate representation of zero.
func nonNegative[Shape any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonNegative})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Shape](), Constraints: constraints})
}

func registerSessionValues(s *Shapes) {
	nonEmpty[protocol.Session](s, "provider", "model")
	nonEmpty[protocol.GetSessionRequest](s, "sessionId")
	nonEmpty[protocol.DeleteSessionRequest](s, "sessionId")
	nonEmpty[protocol.ForkSessionRequest](s, "sessionId")
	nonEmpty[protocol.RollbackSessionRequest](s, "sessionId")
	nonEmpty[protocol.ExportSessionRequest](s, "sessionId")

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateSessionRequest](),
		Constraints: []FieldConstraint{
			{Field: "sessionId", Kind: ConstraintNonEmpty},
			{Field: "expectedRevision", Kind: ConstraintPositive},
			{Field: "provider", Kind: ConstraintNonEmpty},
			{Field: "model", Kind: ConstraintNonEmpty},
		},
	})

	// Import is RESTORE semantics — the session comes back under the id it was
	// exported with — so an artifact with no id names no session to restore.
	nonEmpty[protocol.ImportSessionRequest](s, "artifact.session.id")
}

func registerArtifactValues(s *Shapes) {
	nonEmpty[protocol.ArtifactSession](s, "provider", "model")
	// Import accepts exactly the archive revision this development build emits.
	// Publishing the version as an unconstrained integer would make generated
	// clients promise support the runtime deliberately refuses.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SessionArtifact](),
		Constraints: []FieldConstraint{
			{Field: "version", Kind: ConstraintMinimum, Limit: protocol.SessionArtifactVersion},
			{Field: "version", Kind: ConstraintMaximum, Limit: protocol.SessionArtifactVersion},
		},
	})
	nonNegative[protocol.ArtifactRun](s, "messageMark", "contextTokens")
	nonNegative[protocol.ArtifactRunMetrics](s, "steps", "activeDurationMillis")
	nonNegative[protocol.ArtifactUsage](s,
		"inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens", "costUsd",
	)
	nonNegative[protocol.ArtifactModelUsage](s,
		"inputTokens", "outputTokens", "cacheReadTokens", "cacheWriteTokens", "reasoningTokens", "costUsd",
	)
	nonNegative[protocol.ArtifactItem](s, "droppedMessages", "durationMillis")
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.ArtifactProblem](),
		Constraints: []FieldConstraint{{Field: "retryAfterSeconds", Kind: ConstraintPositive}},
	})
}

func registerRunValues(s *Shapes) {
	nonNegative[protocol.Item](s, "durationMillis")
	nonNegative[protocol.RunRef](s, "contextTokens")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunProtocolProfile](),
		Constraints: []FieldConstraint{
			{Field: "requiredFeatures", Kind: ConstraintUniqueItems},
			{Field: "interruptTypes", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.StartRunRequest](),
		Constraints: []FieldConstraint{
			{Field: "sessionId", Kind: ConstraintNonEmpty},
			{Field: "input", Kind: ConstraintNonEmptyItems},
			{Field: "maxTotalTokens", Kind: ConstraintNonNegative},
			{Field: "maxSteps", Kind: ConstraintNonNegative},
			{Field: "maxBudgetUsd", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GenerationParams](),
		Constraints: []FieldConstraint{
			{Field: "temperature", Kind: ConstraintNonNegative},
			{Field: "temperature", Kind: ConstraintMaximum, Limit: 2},
			{Field: "maxTokens", Kind: ConstraintPositive},
			{Field: "topP", Kind: ConstraintNonNegative},
			{Field: "topP", Kind: ConstraintMaximum, Limit: 1},
			{Field: "stop", Kind: ConstraintNonEmptyItems},
			{Field: "stop", Kind: ConstraintUniqueItems},
		},
	})
	for _, limits := range []any{protocol.RunLimits{}, protocol.ArtifactRunLimits{}} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: reflect.TypeOf(limits),
			Constraints: []FieldConstraint{
				{Field: "maxTotalTokens", Kind: ConstraintNonNegative},
				{Field: "maxSteps", Kind: ConstraintNonNegative},
				{Field: "maxBudgetUsd", Kind: ConstraintNonNegative},
			},
		})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ResumeRunRequest](),
		Constraints: []FieldConstraint{
			{Field: "runId", Kind: ConstraintNonEmpty},
			{Field: "input", Kind: ConstraintNonEmptyItems},
		},
	})
	for _, questionType := range []reflect.Type{
		typeOf[protocol.Question](),
		typeOf[protocol.ArtifactQuestion](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType:      questionType,
			Constraints: []FieldConstraint{{Field: "fields", Kind: ConstraintNonEmptyItems}},
		})
	}
	for _, fieldType := range []reflect.Type{
		typeOf[protocol.QuestionField](),
		typeOf[protocol.ArtifactQuestionField](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType: fieldType,
			Constraints: []FieldConstraint{
				{Field: "prompt", Kind: ConstraintNonEmpty},
				{Field: "header", Kind: ConstraintMaxLength, Limit: 12},
				{Field: "options", Kind: ConstraintMinItems, Limit: 2},
			},
		})
	}
	for _, optionType := range []reflect.Type{
		typeOf[protocol.QuestionOption](),
		typeOf[protocol.ArtifactQuestionOption](),
	} {
		s.valueConstraint(FieldConstraintSpec{
			GoType:      optionType,
			Constraints: []FieldConstraint{{Field: "label", Kind: ConstraintNonEmpty}},
		})
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.InterruptResponseValue](),
		Constraints: []FieldConstraint{{Field: "answers", Kind: ConstraintNonEmptyItems}},
	})
	nonEmpty[protocol.StartRunResponse](s, "runId", "segmentId", "userItemId")
	nonEmpty[protocol.ResumeRunResponse](s, "runId", "segmentId", "userItemId")
	// Subscribe and steer both address a SEGMENT: naming only the run would let the
	// runtime pick whichever one is live, which is how a client silently ends up
	// folding — or steering — an execution it never saw.
	nonEmpty[protocol.SubscribeRunRequest](s, "runId", "segmentId")
	nonEmpty[protocol.GetRunRequest](s, "runId")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.CancelRunRequest](),
		Constraints: []FieldConstraint{
			{Field: "runId", Kind: ConstraintNonEmpty},
			{Field: "reason", Kind: ConstraintMaxLength, Limit: runs.MaxCancellationReasonCharacters},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SteerRunRequest](),
		Constraints: []FieldConstraint{
			{Field: "runId", Kind: ConstraintNonEmpty},
			{Field: "expectedSegmentId", Kind: ConstraintNonEmpty},
			{Field: "input", Kind: ConstraintNonEmptyItems},
		},
	})
	// The scope is required and its tag decides everything else about the read, so a
	// scope with no tag is a request that never said what it wanted.
	nonEmpty[protocol.ListItemsRequest](s, "scope.type")
	// An omitted status filter already means "every status", so an empty array is
	// the one thing it cannot mean, and a repeat asks a set for something a set
	// does not have.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ListRunsRequest](),
		Constraints: []FieldConstraint{
			{Field: "statuses", Kind: ConstraintNonEmptyItems},
			{Field: "statuses", Kind: ConstraintUniqueItems},
		},
	})
}

func registerPlanValues(s *Shapes) {
	nonEmpty[protocol.GetPlanRequest](s, "sessionId")
}

func registerWorkspaceValues(s *Shapes) {
	nonEmpty[protocol.WorkspaceRef](s, "path")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetDiffRequest](),
		Constraints: []FieldConstraint{
			{Field: "limit", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GetFileHeadRequest](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "lines", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ReadFileRequest](),
		Constraints: []FieldConstraint{
			{Field: "path", Kind: ConstraintNonEmpty},
			{Field: "startLine", Kind: ConstraintNonNegative},
			{Field: "endLine", Kind: ConstraintNonNegative},
			{Field: "maxBytes", Kind: ConstraintNonNegative},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GrepRequest](),
		Constraints: []FieldConstraint{
			{Field: "query", Kind: ConstraintNonEmpty},
			{Field: "limit", Kind: ConstraintNonNegative},
		},
	})
}

func registerUsageValues(s *Shapes) {
	nonEmpty[protocol.SessionUsageRequest](s, "sessionId")
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.UsageSummaryRequest](),
		Constraints: []FieldConstraint{{Field: "sinceDays", Kind: ConstraintNonNegative}},
	})
}

func registerSkillValues(s *Shapes) {
	nonEmpty[protocol.SkillNameRequest](s, "name")
	nonEmpty[protocol.SkillProposalRef](s, "name", "revision")
}

func registerHookValues(s *Shapes) {
	nonEmpty[protocol.SetHookTrustRequest](s, "projectRoot")
}

func registerApprovalValues(s *Shapes) {
	nonEmpty[protocol.ListApprovalRulesRequest](s, "sessionId")
	nonEmpty[protocol.ForgetApprovalRuleRequest](s, "id")
}

func registerMCPValues(s *Shapes) {
	nonEmpty[protocol.MCPServerRequest](s, "server")
	nonEmpty[protocol.CreateMCPAuthorizationAttemptRequest](s, "server")
	nonEmpty[protocol.MCPAuthorizationAttemptRequest](s, "attemptId")
	nonEmpty[protocol.MCPAuthorizationAttempt](s, "id", "server")
	nonEmpty[protocol.MCPConnection](s, "url", "command")
	nonEmpty[protocol.MCPConnectionInput](s, "url", "command")
	nonEmpty[protocol.MCPAuthorizationChange](s, "value")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPHeadersChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPEnvironmentChange](),
		Constraints: []FieldConstraint{
			{Field: "value", Kind: ConstraintNonEmptyProperties},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.MCPServerCandidate](),
		Constraints: []FieldConstraint{
			{Field: "name", Kind: ConstraintNonEmpty},
			{Field: "timeoutSeconds", Kind: ConstraintNonNegative},
			{Field: "disabledTools", Kind: ConstraintUniqueItems},
			{Field: "autoApproveTools", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateMCPServerRequest](),
		Constraints: []FieldConstraint{
			{Field: "server", Kind: ConstraintNonEmpty},
			{Field: "timeoutSeconds", Kind: ConstraintNonNegative},
			{Field: "disabledTools", Kind: ConstraintUniqueItems},
			{Field: "autoApproveTools", Kind: ConstraintUniqueItems},
		},
	})
}

func registerProviderValues(s *Shapes) {
	nonEmpty[protocol.UpdateProviderRequest](s, "provider")
	nonEmpty[protocol.ProviderConfigChange](s, "value")
	nonEmpty[protocol.TestProviderRequest](s, "provider")
}

func registerToolValues(s *Shapes) {
	nonEmpty[protocol.InvokeToolRequest](s, "name")
}

func registerKnowledgeValues(s *Shapes) {
	nonEmpty[protocol.KnowledgeEntry](s, "revision")
	nonEmpty[protocol.UpdateKnowledgeRequest](s, "expectedRevision")
}

func registerAgentMemoryValues(s *Shapes) {
	nonEmpty[protocol.AgentMemoryItemRequest](s, "id")
	nonEmpty[protocol.AgentMemoryReviewRequest](s, "id")
	nonEmpty[protocol.AgentMemoryUpdateRequest](s, "id")
	nonEmpty[protocol.AgentMemoryAddRequest](s, "content")
}

func registerScheduleValues(s *Shapes) {
	nonEmpty[protocol.CreateScheduleRequest](s, "instructions", "cron")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateScheduleRequest](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "expectedRevision", Kind: ConstraintPositive},
			{Field: "instructions", Kind: ConstraintNonEmpty},
			{Field: "cron", Kind: ConstraintNonEmpty},
		},
	})
	nonEmpty[protocol.DeleteScheduleRequest](s, "id")
	nonEmpty[protocol.RunScheduleNowRequest](s, "id")
}

func registerGoalValues(s *Shapes) {
	nonEmpty[protocol.StartGoalRequest](s, "sessionId", "objective")
	nonEmpty[protocol.GoalRequest](s, "sessionId")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.GoalBudget](),
		Constraints: []FieldConstraint{
			{Field: "maxRuns", Kind: ConstraintNonNegative},
			{Field: "maxCostUsd", Kind: ConstraintNonNegative},
			{Field: "maxSteps", Kind: ConstraintNonNegative},
		},
	})
}

func registerRuntimeValues(s *Shapes) {
	nonEmpty[protocol.ClientInfo](s, "name", "version")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ClientCapabilities](),
		Constraints: []FieldConstraint{
			{Field: "interruptTypes", Kind: ConstraintUniqueItems},
			{Field: "excludedEphemeralEvents", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.MCPAuthorizationAttemptLimits](),
		Constraints: []FieldConstraint{{Field: "retentionSeconds", Kind: ConstraintPositive}},
	})

	// A subscription names a set. Absence and an empty set both describe no stream,
	// while duplicates claim a set distinction that does not exist.
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RuntimeSubscribeRequest](),
		Constraints: []FieldConstraint{
			{Field: "topics", Kind: ConstraintNonEmptyItems},
			{Field: "topics", Kind: ConstraintUniqueItems},
		},
	})

	// Sequence zero is the sentinel before the hub assigns a frame. Every array is
	// a narrowing set: when present it names at least one unique resource. Variant
	// registration separately decides which sets are required or forbidden.
	eventConstraints := []FieldConstraint{{Field: "sequence", Kind: ConstraintPositive}}
	for _, field := range []string{
		"paths", "names", "serverIds", "scheduleIds", "sessionIds", "runIds",
		"topics", "watchIds",
	} {
		eventConstraints = append(eventConstraints,
			FieldConstraint{Field: field, Kind: ConstraintNonEmptyItems},
			FieldConstraint{Field: field, Kind: ConstraintUniqueItems},
		)
	}
	s.valueConstraint(FieldConstraintSpec{
		GoType:      typeOf[protocol.RuntimeEvent](),
		Constraints: eventConstraints,
	})

	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.PendingInterruptSet](),
		Constraints: []FieldConstraint{
			{Field: "rootRunId", Kind: ConstraintNonEmpty},
			{Field: "sessionId", Kind: ConstraintNonEmpty},
			{Field: "interrupts", Kind: ConstraintNonEmptyItems},
		},
	})
	// A set is owned by its root, while every Interrupt names the Run that raised
	// it. Empty ids satisfy JSON's string type but identify neither resource, so
	// both the live segment outcome and cold interrupt read must reject them.
	nonEmpty[protocol.Interrupt](s, "itemId", "runId")
	// Structured problems are useful only when their leaves identify something.
	// Register the leaf types as validation roots too, so ValidateWireTree applies
	// their string and enum constraints when they are nested in ProblemData.
	nonEmpty[protocol.ActiveRunRef](s, "runId")
	nonEmpty[protocol.CapabilityRequirement](s, "name")
	nonEmpty[protocol.FieldError](s, "field", "detail")
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ProblemData](),
		Constraints: []FieldConstraint{
			{Field: "retryAfterSeconds", Kind: ConstraintPositive},
			{Field: "requiredCapabilities", Kind: ConstraintNonEmptyItems},
			{Field: "requiredCapabilities", Kind: ConstraintUniqueItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.RunReplayLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxEvents", Kind: ConstraintPositive},
			{Field: "maxBytes", Kind: ConstraintPositive},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.IdempotencyLimits](),
		Constraints: []FieldConstraint{
			{Field: "retentionSeconds", Kind: ConstraintPositive},
			{Field: "namespace", Kind: ConstraintNonEmpty},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.SubscriptionLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxTopics", Kind: ConstraintPositive},
			{Field: "maxWatches", Kind: ConstraintPositive},
		},
	})
}
