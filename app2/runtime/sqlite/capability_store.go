package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

var ErrCapabilityNotFound = errors.New("sqlite: capability resource not found")

func (database *Database) ListManagedSkillRecords(ctx context.Context) ([]protocol.ManagedSkill, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body, archived FROM managed_skills ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]protocol.ManagedSkill, 0)
	for rows.Next() {
		var body string
		var archived bool
		if err := rows.Scan(&body, &archived); err != nil {
			return nil, err
		}
		var value protocol.ManagedSkill
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, err
		}
		value.Lifecycle = protocol.SkillLifecycleActive
		if archived {
			value.Lifecycle = protocol.SkillLifecycleArchived
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) PutManagedSkill(ctx context.Context, value protocol.ManagedSkill) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	archived := value.Lifecycle == protocol.SkillLifecycleArchived
	_, err = database.database.ExecContext(
		ctx,
		`INSERT INTO managed_skills(name,archived,body,updated_at) VALUES(?,?,?,?)
		 ON CONFLICT(name) DO UPDATE SET archived=excluded.archived,body=excluded.body,updated_at=excluded.updated_at`,
		value.Name,
		archived,
		string(body),
		encodeTime(time.Now()),
	)
	return err
}

func (database *Database) DeleteManagedSkill(ctx context.Context, name string) error {
	_, err := database.database.ExecContext(ctx, `DELETE FROM managed_skills WHERE name=?`, name)
	return err
}

func (database *Database) ListSkillProposalRecords(ctx context.Context, workspace string) ([]protocol.SkillProposal, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body FROM skill_proposals WHERE workspace_path=? ORDER BY name`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]protocol.SkillProposal, 0)
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var value protocol.SkillProposal
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (database *Database) GetSkillProposalRecord(ctx context.Context, workspace, name, revision string) (protocol.SkillProposal, error) {
	var body string
	err := database.database.QueryRowContext(
		ctx,
		`SELECT body FROM skill_proposals WHERE workspace_path=? AND name=? AND revision=?`,
		workspace,
		name,
		revision,
	).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.SkillProposal{}, ErrCapabilityNotFound
	}
	if err != nil {
		return protocol.SkillProposal{}, err
	}
	var value protocol.SkillProposal
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		return protocol.SkillProposal{}, err
	}
	return value, nil
}

func (database *Database) PutSkillProposalRecord(ctx context.Context, workspace string, value protocol.SkillProposal) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = database.database.ExecContext(
		ctx,
		`INSERT INTO skill_proposals(workspace_path,name,revision,body,updated_at) VALUES(?,?,?,?,?)
		 ON CONFLICT(workspace_path,name) DO UPDATE SET revision=excluded.revision,body=excluded.body,updated_at=excluded.updated_at`,
		workspace,
		value.Name,
		value.Revision,
		string(body),
		encodeTime(time.Now()),
	)
	return err
}

func (database *Database) DeleteSkillProposalRecord(ctx context.Context, workspace, name, revision string) error {
	result, err := database.database.ExecContext(
		ctx,
		`DELETE FROM skill_proposals WHERE workspace_path=? AND name=? AND revision=?`,
		workspace,
		name,
		revision,
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrCapabilityNotFound
	}
	return nil
}
