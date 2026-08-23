package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/sessionflow"
)

func (database *Database) CreateSessionFork(
	ctx context.Context,
	write sessionflow.ForkWrite,
) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := insertSessionTx(ctx, transaction, write.Session); err != nil {
		return err
	}
	if err := insertMaterialRuns(ctx, transaction, write.Runs); err != nil {
		return err
	}
	if err := insertPlanBoundaries(ctx, transaction, write.PlanBoundaries); err != nil {
		return err
	}
	for _, record := range write.Items {
		if err := insertItem(ctx, transaction, record); err != nil {
			return err
		}
	}
	for _, record := range write.Messages {
		if err := insertConversationMessage(ctx, transaction, record); err != nil {
			return err
		}
	}
	for _, record := range write.ToolResults {
		if err := insertToolResult(ctx, transaction, record); err != nil {
			return err
		}
	}
	if write.Plan != nil {
		if err := insertPlanTx(ctx, transaction, *write.Plan); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (database *Database) RollbackSessionHistory(
	ctx context.Context,
	write sessionflow.RollbackWrite,
) (session.Session, error) {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return session.Session{}, err
	}
	defer transaction.Rollback()
	var openRuns int
	if err := transaction.QueryRowContext(ctx, `
		SELECT count(*) FROM runs WHERE session_id=? AND status!='finished'`,
		write.SessionID.String(),
	).Scan(&openRuns); err != nil {
		return session.Session{}, err
	}
	if openRuns > 0 {
		return session.Session{}, protocol.ErrSessionBusy
	}
	if write.Plan != nil {
		if err := savePlanCAS(ctx, transaction, *write.Plan, write.ExpectedPlanRevision); err != nil {
			return session.Session{}, err
		}
	}
	for _, id := range write.DropRootRunIDs {
		result, err := transaction.ExecContext(ctx, `
			DELETE FROM runs WHERE id=? AND session_id=? AND parent_run_id IS NULL`,
			id,
			write.SessionID.String(),
		)
		if err != nil {
			return session.Session{}, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return session.Session{}, err
		}
		if changed != 1 {
			return session.Session{}, protocol.ErrRunNotFound
		}
	}
	if _, err := transaction.ExecContext(
		ctx,
		`DELETE FROM goals WHERE session_id=?`,
		write.SessionID.String(),
	); err != nil {
		return session.Session{}, err
	}
	if _, err := transaction.ExecContext(
		ctx,
		`DELETE FROM plan_modes WHERE session_id=?`,
		write.SessionID.String(),
	); err != nil {
		return session.Session{}, err
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE sessions SET revision=revision+1,updated_at=? WHERE id=?`,
		encodeTime(write.Now),
		write.SessionID.String(),
	)
	if err != nil {
		return session.Session{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return session.Session{}, err
	}
	if changed != 1 {
		return session.Session{}, session.ErrNotFound
	}
	value, err := scanSession(transaction.QueryRowContext(ctx, `
		SELECT id,title,workspace_path,provider,model,favorite,revision,created_at,updated_at
		FROM sessions WHERE id=?`, write.SessionID.String()))
	if err != nil {
		return session.Session{}, err
	}
	if err := transaction.Commit(); err != nil {
		return session.Session{}, err
	}
	return value, nil
}

func (database *Database) CreateImportedSession(
	ctx context.Context,
	write sessionflow.ImportWrite,
) error {
	material := write.Material
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if err := insertSessionTx(ctx, transaction, material.Session); err != nil {
		return err
	}
	if err := insertMaterialRuns(ctx, transaction, material.Runs); err != nil {
		return err
	}
	if err := insertPlanBoundaries(ctx, transaction, material.PlanBoundaries); err != nil {
		return err
	}
	for _, record := range material.Items {
		if err := insertItem(ctx, transaction, record); err != nil {
			return err
		}
	}
	for _, record := range material.Messages {
		if err := insertConversationMessage(ctx, transaction, record); err != nil {
			return err
		}
	}
	for _, record := range material.ToolResults {
		if err := insertToolResult(ctx, transaction, record); err != nil {
			return err
		}
	}
	if err := insertPlanTx(ctx, transaction, material.Plan); err != nil {
		return err
	}
	return transaction.Commit()
}

func insertMaterialRuns(
	ctx context.Context,
	transaction *sql.Tx,
	records []rundomain.Record,
) error {
	for _, record := range records {
		if err := insertRun(ctx, transaction, record); err != nil {
			return err
		}
	}
	return nil
}

func insertPlanBoundaries(
	ctx context.Context,
	transaction *sql.Tx,
	boundaries map[string]plandomain.Boundary,
) error {
	for runID, boundary := range boundaries {
		body, err := encodePlanBoundary(boundary)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO plan_boundaries(run_id,body) VALUES(?,?)`,
			runID,
			body,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertSessionTx(
	ctx context.Context,
	transaction *sql.Tx,
	value session.Session,
) error {
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO sessions(
			id,title,workspace_path,provider,model,favorite,revision,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
		value.ID().String(),
		value.Title(),
		value.Workspace().Path(),
		value.Selection().Provider(),
		value.Selection().Model(),
		value.Favorite(),
		value.Revision(),
		encodeTime(value.CreatedAt()),
		encodeTime(value.UpdatedAt()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert session %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return session.ErrRevisionConflict
	}
	return nil
}

func insertPlanTx(
	ctx context.Context,
	transaction *sql.Tx,
	value plandomain.State,
) error {
	if value.Revision() == 0 {
		return nil
	}
	return savePlanCAS(ctx, transaction, value, 0)
}
