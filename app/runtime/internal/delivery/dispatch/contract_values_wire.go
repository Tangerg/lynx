package dispatch

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// The value constraints of every request shape.
//
// They were thirty hand-written Validate() methods, which meant the schema had no
// minLength anywhere and a generated TypeScript validator would have had nothing to
// read — the Go check was the only statement, so "the three agree" was unverifiable
// by construction. Declared here, one statement generates all three.
//
// Only two kinds appear because only two exist: a string that may not be empty and
// a number that may not be zero. Closed-enum membership is derived from the enum's
// own declared value set, not restated here.

func registerValueConstraints(s *Shapes) {
	registerSessionValues(s)
	registerRunValues(s)
	registerWorkspaceValues(s)
	registerIntegrationValues(s)
	registerMemoryValues(s)
	registerScheduleValues(s)
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
	nonEmpty[protocol.ResumeRunRequest](s, "runId")
	nonEmpty[protocol.SubscribeRunRequest](s, "runId")
	nonEmpty[protocol.GetRunRequest](s, "runId")
	nonEmpty[protocol.CancelRunRequest](s, "runId")
	nonEmpty[protocol.SteerRunRequest](s, "runId", "message")
	nonEmpty[protocol.ListItemsRequest](s, "sessionId")
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
