package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/accounting"
	feedbackdomain "github.com/Tangerg/lynx/app2/runtime/domain/feedback"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type storedUsageRunFacts struct {
	Metrics protocol.RunMetrics `json:"metrics"`
}

func (database *Database) ListUsageRunRecords(
	ctx context.Context,
	sessionID string,
	since time.Time,
) ([]accounting.RunRecord, error) {
	query := `SELECT session_id,provider,model,body,finished_at
		FROM runs WHERE status='finished'`
	arguments := make([]any, 0, 2)
	if sessionID != "" {
		query += ` AND session_id=?`
		arguments = append(arguments, sessionID)
	}
	if !since.IsZero() {
		query += ` AND finished_at>=?`
		arguments = append(arguments, encodeTime(since))
	}
	query += ` ORDER BY finished_at,id`
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]accounting.RunRecord, 0)
	for rows.Next() {
		var value accounting.RunRecord
		var body, finishedAt string
		if err := rows.Scan(
			&value.SessionID,
			&value.Provider,
			&value.Model,
			&body,
			&finishedAt,
		); err != nil {
			return nil, err
		}
		value.FinishedAt, err = decodeTime(finishedAt)
		if err != nil {
			return nil, err
		}
		value.Usage, err = decodeAccountingUsage(body)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func decodeAccountingUsage(body string) (*accounting.Usage, error) {
	var facts storedUsageRunFacts
	if err := json.Unmarshal([]byte(body), &facts); err != nil {
		return nil, fmt.Errorf("sqlite: decode usage Run facts: %w", err)
	}
	if facts.Metrics.Usage == nil {
		return nil, nil
	}
	if err := validateStoredUsage(*facts.Metrics.Usage); err != nil {
		return nil, fmt.Errorf("sqlite: invalid stored usage: %w", err)
	}
	value := &accounting.Usage{
		ModelUsage: accountingUsage(facts.Metrics.Usage.ModelUsage),
	}
	if len(facts.Metrics.Usage.ByModel) > 0 {
		value.ByModel = make(map[string]accounting.ModelUsage, len(facts.Metrics.Usage.ByModel))
		for model, usage := range facts.Metrics.Usage.ByModel {
			value.ByModel[model] = accountingUsage(usage)
		}
	}
	return value, nil
}

func validateStoredUsage(value protocol.Usage) error {
	if err := validateStoredModelUsage(value.ModelUsage); err != nil {
		return err
	}
	for model, usage := range value.ByModel {
		if model == "" {
			return errors.New("empty model identity")
		}
		if err := validateStoredModelUsage(usage); err != nil {
			return fmt.Errorf("model %q: %w", model, err)
		}
	}
	return nil
}

func validateStoredModelUsage(value protocol.ModelUsage) error {
	if value.InputTokens < 0 || value.OutputTokens < 0 ||
		value.CacheReadTokens < 0 || value.CacheWriteTokens < 0 ||
		value.ReasoningTokens < 0 {
		return errors.New("negative token count")
	}
	if value.CostUSD != nil && (*value.CostUSD < 0 || math.IsNaN(*value.CostUSD) || math.IsInf(*value.CostUSD, 0)) {
		return errors.New("invalid cost")
	}
	return nil
}

func accountingUsage(value protocol.ModelUsage) accounting.ModelUsage {
	return accounting.ModelUsage{
		InputTokens:      value.InputTokens,
		OutputTokens:     value.OutputTokens,
		CacheReadTokens:  value.CacheReadTokens,
		CacheWriteTokens: value.CacheWriteTokens,
		ReasoningTokens:  value.ReasoningTokens,
		CostUSD:          cloneFloat(value.CostUSD),
	}
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func (database *Database) SessionExists(ctx context.Context, id string) (bool, error) {
	var one int
	err := database.database.QueryRowContext(
		ctx,
		`SELECT 1 FROM sessions WHERE id=?`,
		id,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (database *Database) ResolveFeedbackAttribution(
	ctx context.Context,
	subject feedbackdomain.Subject,
) (feedbackdomain.Attribution, bool, error) {
	var attribution feedbackdomain.Attribution
	var err error
	switch subject.MostSpecific() {
	case "item":
		err = database.database.QueryRowContext(ctx, `
			SELECT session_id,run_id,id FROM items WHERE id=?`,
			subject.ItemID,
		).Scan(&attribution.SessionID, &attribution.RunID, &attribution.ItemID)
	case "run":
		err = database.database.QueryRowContext(ctx, `
			SELECT session_id,id FROM runs WHERE id=?`,
			subject.RunID,
		).Scan(&attribution.SessionID, &attribution.RunID)
	case "session":
		err = database.database.QueryRowContext(ctx, `
			SELECT id FROM sessions WHERE id=?`,
			subject.SessionID,
		).Scan(&attribution.SessionID)
	default:
		return feedbackdomain.Attribution{}, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return feedbackdomain.Attribution{}, false, nil
	}
	if err != nil {
		return feedbackdomain.Attribution{}, false, err
	}
	return attribution, true, nil
}

func (database *Database) CreateFeedbackRecord(
	ctx context.Context,
	record feedbackdomain.Record,
) error {
	if err := record.Validate(); err != nil {
		return err
	}
	attribution := record.Attribution()
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO feedback(id,session_id,run_id,item_id,rating,text,created_at)
		VALUES(?,?,?,?,?,?,?)`,
		record.ID(),
		nullableString(attribution.SessionID),
		nullableString(attribution.RunID),
		nullableString(attribution.ItemID),
		record.Rating(),
		record.Text(),
		encodeTime(record.CreatedAt()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create feedback: %w", err)
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
