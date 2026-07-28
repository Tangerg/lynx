package protocol

import "strings"

// Validator is implemented by a request DTO that carries a constraint its JSON
// shape alone cannot express: an id that must be present, a revision that must
// be positive, an enum that must be one of a closed set.
//
// Delivery calls Validate immediately after decoding and before the request
// reaches any use case, mapping a failure to invalid_params (API.md §8.2). A
// constraint therefore belongs HERE and not in a handler: it is a property of
// the request shape, so every transport and every generated client reads one
// statement of it. A handler that re-checks the same field is a second author.
//
// Validate must stay a pure function of the value — no storage, no dispatcher,
// no executor (contract §11.2 / §14.6 gate 7). "Does this session exist" is not
// a shape constraint and is answered by the use case.
type Validator interface {
	Validate() error
}

// ConstraintError reports which fields of a request violated their constraints.
// It is what makes [ProblemData.Errors] answerable: a validation failure knows
// the offending params key, so delivery can hand the client a per-field list to
// flag instead of one prose sentence it would have to parse (API.md §8.3).
//
// Detail strings are programmer diagnostics, not UI copy — a client renders its
// own localized message keyed by field + type (the §8.2 lookup), exactly as it
// does for a ProblemData.type.
type ConstraintError struct {
	Fields []FieldError
}

func (e *ConstraintError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Detail)
	}
	return strings.Join(parts, "; ")
}

// violate returns nil when there is nothing to report, so a Validate built from
// several checks composes without the caller testing each one.
func violate(fields ...FieldError) error {
	present := make([]FieldError, 0, len(fields))
	for _, f := range fields {
		if f.Field != "" {
			present = append(present, f)
		}
	}
	if len(present) == 0 {
		return nil
	}
	return &ConstraintError{Fields: present}
}

func required(field, value string) FieldError {
	if value == "" {
		return FieldError{Field: field, Detail: "is required"}
	}
	return FieldError{}
}

func positive(field string, value uint64) FieldError {
	if value == 0 {
		return FieldError{Field: field, Detail: "must be greater than zero"}
	}
	return FieldError{}
}

// ─── Sessions ───────────────────────────────────────────────────────

func (r GetSessionRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r DeleteSessionRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r ForkSessionRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r RollbackSessionRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r ExportSessionRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r UpdateSessionRequest) Validate() error {
	return violate(
		required("sessionId", r.SessionID),
		positive("expectedRevision", r.ExpectedRevision),
	)
}

// Validate requires the artifact's own session id: import is RESTORE semantics
// (the session comes back under the id it was exported with), so an artifact
// with no id names no session to restore.
func (r ImportSessionRequest) Validate() error {
	return violate(required("artifact.session.id", r.Artifact.Session.ID))
}

// ─── Runs / Items ───────────────────────────────────────────────────

func (r ResumeRunRequest) Validate() error { return violate(required("runId", r.RunID)) }

func (r SubscribeRunRequest) Validate() error { return violate(required("runId", r.RunID)) }

func (r CancelRunRequest) Validate() error { return violate(required("runId", r.RunID)) }

func (r SteerRunRequest) Validate() error {
	return violate(required("runId", r.RunID), required("message", r.Message))
}

func (r ListItemsRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

// ─── Workspace ──────────────────────────────────────────────────────

func (r GetFileHeadRequest) Validate() error { return violate(required("path", r.Path)) }

func (r ReadFileRequest) Validate() error { return violate(required("path", r.Path)) }

func (r GrepRequest) Validate() error { return violate(required("query", r.Query)) }

// ─── Skills / Hooks ─────────────────────────────────────────────────

func (r SkillNameRequest) Validate() error { return violate(required("name", r.Name)) }

func (r SetHookTrustRequest) Validate() error { return violate(required("projectRoot", r.ProjectRoot)) }

// ─── MCP ────────────────────────────────────────────────────────────

func (r MCPServerRequest) Validate() error { return violate(required("server", r.Server)) }

func (r ConfigureMCPServerRequest) Validate() error { return violate(required("name", r.Name)) }

func (r RemoveMCPServerRequest) Validate() error { return violate(required("name", r.Name)) }

func (r SetMCPEnabledRequest) Validate() error { return violate(required("name", r.Name)) }

// ─── Providers / Models / Tools / Usage ─────────────────────────────

func (r ConfigureProviderRequest) Validate() error { return violate(required("provider", r.Provider)) }

func (r TestProviderRequest) Validate() error { return violate(required("provider", r.Provider)) }

func (r InvokeToolRequest) Validate() error { return violate(required("name", r.Name)) }

func (r SessionUsageRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

// ─── Memory / Agent memory ──────────────────────────────────────────

func (r GetMemoryRequest) Validate() error { return validMemoryScope(r.Scope) }

func (r UpdateMemoryRequest) Validate() error { return validMemoryScope(r.Scope) }

func validMemoryScope(scope MemoryScope) error {
	if scope.Valid() {
		return nil
	}
	return violate(FieldError{
		Field:  "scope",
		Detail: `must be "cwd", "projectRoot", or "home"`,
	})
}

func (r AgentMemoryItemRequest) Validate() error { return violate(required("id", r.ID)) }

func (r AgentMemoryReviewRequest) Validate() error { return violate(required("id", r.ID)) }

func (r AgentMemoryUpdateRequest) Validate() error { return violate(required("id", r.ID)) }

func (r AgentMemoryAddRequest) Validate() error { return violate(required("content", r.Content)) }

// ─── Schedules / Goals / Codebase ───────────────────────────────────

func (r UpdateScheduleRequest) Validate() error {
	return violate(required("id", r.ID), positive("expectedRevision", r.ExpectedRevision))
}

func (r DeleteScheduleRequest) Validate() error { return violate(required("id", r.ID)) }

func (r RunScheduleNowRequest) Validate() error { return violate(required("id", r.ID)) }

func (r StartGoalRequest) Validate() error {
	return violate(required("sessionId", r.SessionID), required("objective", r.Objective))
}

func (r GoalRequest) Validate() error { return violate(required("sessionId", r.SessionID)) }

func (r CodebaseSearchRequest) Validate() error { return violate(required("query", r.Query)) }
