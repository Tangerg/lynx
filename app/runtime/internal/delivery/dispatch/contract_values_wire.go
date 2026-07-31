package dispatch

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

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
	registerSessionValues(s)
	registerRunValues(s)
	registerWorkspaceValues(s)
	registerIntegrationValues(s)
	registerMemoryValues(s)
	registerScheduleValues(s)
	registerRuntimeValues(s)
}

// nonEmpty builds the common spec: these fields are ids or text that must be there.
func nonEmpty[Request any](s *Shapes, fields ...string) {
	constraints := make([]FieldConstraint, 0, len(fields))
	for _, field := range fields {
		constraints = append(constraints, FieldConstraint{Field: field, Kind: ConstraintNonEmpty})
	}
	s.valueConstraint(FieldConstraintSpec{GoType: typeOf[Request](), Constraints: constraints})
}

func registerSessionValues(s *Shapes) {
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
		},
	})

	// Import is RESTORE semantics — the session comes back under the id it was
	// exported with — so an artifact with no id names no session to restore.
	nonEmpty[protocol.ImportSessionRequest](s, "artifact.session.id")
}

func registerRunValues(s *Shapes) {
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
			{Field: "input", Kind: ConstraintNonEmptyItems},
		},
	})
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ResumeRunRequest](),
		Constraints: []FieldConstraint{
			{Field: "runId", Kind: ConstraintNonEmpty},
			{Field: "input", Kind: ConstraintNonEmptyItems},
		},
	})
	nonEmpty[protocol.StartRunResponse](s, "runId", "segmentId", "userItemId")
	nonEmpty[protocol.ResumeRunResponse](s, "runId", "segmentId", "userItemId")
	// Subscribe and steer both address a SEGMENT: naming only the run would let the
	// runtime pick whichever one is live, which is how a client silently ends up
	// folding — or steering — an execution it never saw.
	nonEmpty[protocol.SubscribeRunRequest](s, "runId", "segmentId")
	nonEmpty[protocol.GetRunRequest](s, "runId")
	nonEmpty[protocol.CancelRunRequest](s, "runId")
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
	nonEmpty[protocol.GetTodosRequest](s, "sessionId")
	nonEmpty[protocol.SessionUsageRequest](s, "sessionId")

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

func registerWorkspaceValues(s *Shapes) {
	nonEmpty[protocol.GetFileHeadRequest](s, "path")
	nonEmpty[protocol.ReadFileRequest](s, "path")
	nonEmpty[protocol.GrepRequest](s, "query")
	nonEmpty[protocol.CodebaseSearchRequest](s, "query")
}

func registerIntegrationValues(s *Shapes) {
	nonEmpty[protocol.SkillNameRequest](s, "name")
	nonEmpty[protocol.SetHookTrustRequest](s, "projectRoot")
	nonEmpty[protocol.MCPServerRequest](s, "server")
	nonEmpty[protocol.ConfigureMCPServerRequest](s, "name")
	nonEmpty[protocol.RemoveMCPServerRequest](s, "name")
	nonEmpty[protocol.SetMCPEnabledRequest](s, "name")
	nonEmpty[protocol.ConfigureProviderRequest](s, "provider")
	nonEmpty[protocol.TestProviderRequest](s, "provider")
	nonEmpty[protocol.InvokeToolRequest](s, "name")
}

func registerMemoryValues(s *Shapes) {
	// GetMemoryRequest / UpdateMemoryRequest carry only `scope`, whose closed set is
	// checked from the enum declaration — nothing to state here.
	nonEmpty[protocol.AgentMemoryItemRequest](s, "id")
	nonEmpty[protocol.AgentMemoryReviewRequest](s, "id")
	nonEmpty[protocol.AgentMemoryUpdateRequest](s, "id")
	nonEmpty[protocol.AgentMemoryAddRequest](s, "content")
}

func registerScheduleValues(s *Shapes) {
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.UpdateScheduleRequest](),
		Constraints: []FieldConstraint{
			{Field: "id", Kind: ConstraintNonEmpty},
			{Field: "expectedRevision", Kind: ConstraintPositive},
		},
	})
	nonEmpty[protocol.DeleteScheduleRequest](s, "id")
	nonEmpty[protocol.RunScheduleNowRequest](s, "id")
	nonEmpty[protocol.StartGoalRequest](s, "sessionId", "objective")
	nonEmpty[protocol.GoalRequest](s, "sessionId")
}

func registerRuntimeValues(s *Shapes) {
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
	s.valueConstraint(FieldConstraintSpec{
		GoType: typeOf[protocol.ProblemData](),
		Constraints: []FieldConstraint{
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
		GoType: typeOf[protocol.SubscriptionLimits](),
		Constraints: []FieldConstraint{
			{Field: "maxTopics", Kind: ConstraintPositive},
			{Field: "maxWatches", Kind: ConstraintPositive},
		},
	})
}
