package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerSessions(r *Registry) {
	Query(r, MethodMeta{Name: "sessions.list", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Session], error) {
			return d.api.ListSessions(ctx, in)
		})

	Query(r, MethodMeta{
		Name:      "sessions.get",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetSessionRequest) (*protocol.Session, error) {
		return d.api.GetSession(ctx, in.SessionID)
	})

	Command(r, MethodMeta{
		Name:      "sessions.create",
		Errors:    []string{protocol.ErrCwdUnavailable.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CreateSessionRequest) (*protocol.Session, error) {
		return d.api.CreateSession(ctx, in)
	})

	// Setting cwd is a relocate, which is its own capability (API.md §9) — hence a
	// conditional rule: the rest of sessions.update stays available when relocate
	// is off, instead of the whole method disappearing.
	Command(r, MethodMeta{
		Name: "sessions.update",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRevisionConflict.Error(),
			protocol.ErrCwdUnavailable.Error(),
		},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "cwd", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureRelocate},
		}},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.UpdateSessionRequest) (*protocol.Session, error) {
		return d.api.UpdateSession(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:      "sessions.delete",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.DeleteSessionRequest) error {
		return d.api.DeleteSession(ctx, in.SessionID)
	})

	Command(r, MethodMeta{
		Name: "sessions.fork",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ForkSessionRequest) (*protocol.Session, error) {
		return d.api.ForkSession(ctx, in)
	})

	// restoreType files/both rewind the working tree from a shadow-git snapshot,
	// which needs features.checkpoints; the default history rollback needs nothing
	// (AUX_API §4.1). Two rules rather than one because the contract states the
	// requirement per value, and a generated schema reads them as two if/then.
	Command(r, MethodMeta{
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
	}, func(d *Dispatcher, ctx context.Context, in protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
		return d.api.RollbackSession(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "sessions.export",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureSessionExport),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ExportSessionRequest) (*protocol.ExportSessionResponse, error) {
		return d.api.ExportSession(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "sessions.import",
		CapabilityRules: requires(protocol.FeatureSessionExport),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ImportSessionRequest) (*protocol.ImportSessionResponse, error) {
		return d.api.ImportSession(ctx, in)
	})
}
