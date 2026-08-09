package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerSessions(registry *Registry) {
	Query(registry, MethodMeta{Name: "sessions.list", Stability: stable},
		func(router *Router, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
			return router.api.ListSessions(ctx, request)
		})

	Query(registry, MethodMeta{
		Name:      "sessions.get",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.GetSessionRequest) (*protocol.Session, error) {
		return router.api.GetSession(ctx, request.SessionID)
	})

	Command(registry, MethodMeta{
		Name:      "sessions.create",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.CreateSessionRequest) (*protocol.Session, error) {
		return router.api.CreateSession(ctx, request)
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
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.UpdateSessionRequest) (*protocol.Session, error) {
		return router.api.UpdateSession(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:      "sessions.delete",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.DeleteSessionRequest) error {
		return router.api.DeleteSession(ctx, request.SessionID)
	})

	Command(registry, MethodMeta{
		Name: "sessions.fork",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ForkSessionRequest) (*protocol.Session, error) {
		return router.api.ForkSession(ctx, request)
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
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
		return router.api.RollbackSession(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "sessions.export",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return router.api.ExportSession(ctx, request)
	})

	Command(registry, MethodMeta{
		Name:            "sessions.import",
		CapabilityRules: requires(protocol.FeatureSessionExport),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return router.api.ImportSession(ctx, request)
	})
}
