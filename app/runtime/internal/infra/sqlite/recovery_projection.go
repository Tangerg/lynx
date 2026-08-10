package sqlite

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// ListNonTerminalRuns projects complete durable Run aggregates for application
// recovery. Storage exposes facts only; it does not decide which Run tree may
// survive a restart.
func (s *RunStore) ListNonTerminalRuns(ctx context.Context) ([]run.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		   FROM runs AS r
		   `+runReadJoins+`
		  WHERE r.state != ?
		  ORDER BY r.started_at, r.run_id`,
		runStateTerminal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
	}
	defer rows.Close()

	var runs []run.Run
	for rows.Next() {
		run, err := scanRunForRecovery(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal Runs: %w", err)
	}
	return runs, nil
}
