package operation

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerSessions(registry *Registry) {
	Query(registry, MethodMeta{Name: "sessions.list"},
		func(service interface {
			ListSessions(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Session], error)
		}, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
			return service.ListSessions(ctx, request)
		})

	Query(registry, MethodMeta{
		Name:   "sessions.get",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		GetSession(context.Context, string) (*protocol.Session, error)
	}, ctx context.Context, request protocol.GetSessionRequest) (*protocol.Session, error) {
		return service.GetSession(ctx, request.SessionID)
	})

	Query(registry, MethodMeta{
		Name:         "sessions.snapshot",
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		Materializes: []string{"items.list", "runs.list", "interrupts.list", "plan.get", "goals.get"},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, func(service interface {
		GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error)
	}, ctx context.Context, request protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
		return service.GetSessionSnapshot(ctx, request)
	})

	Command(registry, MethodMeta{
		Name:   "sessions.create",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		CreateSession(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
		return service.CreateSession(ctx, request)
	})

	// Setting workspace is a relocate, which is its own capability (API.md §9) — hence a
	// conditional rule: the rest of sessions.update stays available when relocate
	// is off, instead of the whole method disappearing.
	Command(registry, MethodMeta{
		Name: "sessions.update",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRevisionConflict.Error(),
			protocol.ErrWorkspaceUnavailable.Error(),
		},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "workspace", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureRelocate},
		}},
	}, func(service interface {
		UpdateSession(context.Context, protocol.UpdateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		return service.UpdateSession(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:   "sessions.delete",
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		DeleteSession(context.Context, string) error
	}, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return service.DeleteSession(ctx, request.SessionID)
	})

	Command(registry, MethodMeta{
		Name: "sessions.fork",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
	}, func(service interface {
		ForkSession(context.Context, protocol.ForkSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.ForkSessionRequest) (*protocol.Session, error) {
		return service.ForkSession(ctx, request)
	})

	// restoreType files/both rewind the working tree from a shadow-git snapshot,
	// which needs features.checkpoints; the default history rollback needs nothing
	// (AUX_API §4.1). Two rules rather than one because the contract states the
	// requirement per value, and a generated schema reads them as two if/then.
	Command(registry, MethodMeta{
		Name: "sessions.rollback",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCheckpointUnavailable.Error(),
			protocol.ErrRevisionConflict.Error(),
		},
		CapabilityRules: []CapabilityRule{
			{
				When:     []FieldCondition{{Field: "restoreType", Operator: OperatorEquals, Value: string(protocol.RestoreFiles)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
			{
				When:     []FieldCondition{{Field: "restoreType", Operator: OperatorEquals, Value: string(protocol.RestoreBoth)}},
				Requires: []string{protocol.FeatureCheckpoints},
			},
		},
	}, func(service interface {
		RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error)
	}, ctx context.Context, request protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
		return service.RollbackSession(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "sessions.export",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
		},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ExportSession(context.Context, protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error)
	}, ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return service.ExportSession(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "sessions.import",
		Errors: []string{
			protocol.ErrRevisionConflict.Error(),
			protocol.ErrWorkspaceUnavailable.Error(),
		},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ImportSession(context.Context, protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error)
	}, ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return service.ImportSession(ctx, request)
	})
}
