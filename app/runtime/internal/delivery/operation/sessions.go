package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	SessionsList     Name = "sessions.list"
	SessionsGet      Name = "sessions.get"
	SessionsSnapshot Name = "sessions.snapshot"
	SessionsCreate   Name = "sessions.create"
	SessionsUpdate   Name = "sessions.update"
	SessionsDelete   Name = "sessions.delete"
	SessionsFork     Name = "sessions.fork"
	SessionsRollback Name = "sessions.rollback"
	SessionsExport   Name = "sessions.export"
	SessionsImport   Name = "sessions.import"
)

func registerSessions(registry *Registry) {
	registry.Query(MethodMeta{Name: SessionsList},
		func(service interface {
			ListSessions(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Session], error)
		}, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
			return service.ListSessions(ctx, request)
		})

	registry.Query(MethodMeta{
		Name:   SessionsGet,
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		GetSession(context.Context, string) (*protocol.Session, error)
	}, ctx context.Context, request protocol.GetSessionRequest) (*protocol.Session, error) {
		return service.GetSession(ctx, request.SessionID)
	})

	registry.Query(MethodMeta{
		Name:         SessionsSnapshot,
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		Materializes: []Name{ItemsList, RunsList, InterruptsList, PlanGet, GoalsGet},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, func(service interface {
		GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error)
	}, ctx context.Context, request protocol.GetSessionSnapshotRequest) (*protocol.SessionSnapshot, error) {
		return service.GetSessionSnapshot(ctx, request)
	})

	registry.Command(MethodMeta{
		Name:   SessionsCreate,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		CreateSession(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
	}, ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
		return service.CreateSession(ctx, request)
	})

	// Setting workspace is a relocate, which is its own capability (API.md §9) — hence a
	// conditional rule: the rest of sessions.update stays available when relocate
	// is off, instead of the whole method disappearing.
	registry.Command(MethodMeta{
		Name: SessionsUpdate,
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

	registry.CommandAck(MethodMeta{
		Name:   SessionsDelete,
		Errors: []string{protocol.ErrSessionNotFound.Error()},
	}, func(service interface {
		DeleteSession(context.Context, string) error
	}, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return service.DeleteSession(ctx, request.SessionID)
	})

	registry.Command(MethodMeta{
		Name: SessionsFork,
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
	registry.Command(MethodMeta{
		Name: SessionsRollback,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCheckpointUnavailable.Error(),
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

	registry.Query(MethodMeta{
		Name:            SessionsExport,
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ExportSession(context.Context, protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error)
	}, ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return service.ExportSession(ctx, request)
	})

	registry.Command(MethodMeta{
		Name:            SessionsImport,
		CapabilityRules: requires(protocol.FeatureSessionExport),
	}, func(service interface {
		ImportSession(context.Context, protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error)
	}, ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return service.ImportSession(ctx, request)
	})
}
