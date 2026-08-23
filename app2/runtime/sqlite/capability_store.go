package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

var ErrCapabilityNotFound = errors.New("sqlite: capability resource not found")

func (database *Database) ListManagedSkillRecords(ctx context.Context) ([]protocol.ManagedSkill, error) {
	rows, err := database.database.QueryContext(ctx, `SELECT body, archived FROM managed_skills ORDER BY name`)
	if err != nil { return nil, err }
	defer rows.Close()
	values:=make([]protocol.ManagedSkill,0)
	for rows.Next() { var body string; var archived bool; if err:=rows.Scan(&body,&archived); err!=nil{return nil,err}; var value protocol.ManagedSkill; if err:=json.Unmarshal([]byte(body),&value);err!=nil{return nil,err}; if archived{value.Lifecycle=protocol.SkillLifecycleArchived}else{value.Lifecycle=protocol.SkillLifecycleActive}; values=append(values,value) }
	return values,rows.Err()
}

func (database *Database) SetManagedSkillLifecycle(ctx context.Context, name string, lifecycle protocol.SkillLifecycle) error {
	archived:=lifecycle==protocol.SkillLifecycleArchived
	result,err:=database.database.ExecContext(ctx,`UPDATE managed_skills SET archived=?, body=json_set(body,'$.lifecycle',?), updated_at=? WHERE name=?`,archived,lifecycle,encodeTime(time.Now()),name)
	if err!=nil{return err}; changed,_:=result.RowsAffected(); if changed==0{return ErrCapabilityNotFound}; return nil
}

func (database *Database) PutManagedSkill(ctx context.Context, value protocol.ManagedSkill) error {
	body,err:=json.Marshal(value);if err!=nil{return err}; archived:=value.Lifecycle==protocol.SkillLifecycleArchived
	_,err=database.database.ExecContext(ctx,`INSERT INTO managed_skills(name,archived,body,updated_at) VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET archived=excluded.archived,body=excluded.body,updated_at=excluded.updated_at`,value.Name,archived,string(body),encodeTime(time.Now()));return err
}

func (database *Database) ListSkillProposalRecords(ctx context.Context, workspace string) ([]protocol.SkillProposal,error){
	rows,err:=database.database.QueryContext(ctx,`SELECT body FROM skill_proposals WHERE workspace_path=? ORDER BY name`,workspace);if err!=nil{return nil,err};defer rows.Close(); values:=make([]protocol.SkillProposal,0);for rows.Next(){var body string;if err:=rows.Scan(&body);err!=nil{return nil,err};var value protocol.SkillProposal;if err:=json.Unmarshal([]byte(body),&value);err!=nil{return nil,err};values=append(values,value)};return values,rows.Err()
}

func (database *Database) GetSkillProposalRecord(ctx context.Context, workspace,name,revision string)(protocol.SkillProposal,error){
	var body string;err:=database.database.QueryRowContext(ctx,`SELECT body FROM skill_proposals WHERE workspace_path=? AND name=? AND revision=?`,workspace,name,revision).Scan(&body);if errors.Is(err,sql.ErrNoRows){return protocol.SkillProposal{},ErrCapabilityNotFound};if err!=nil{return protocol.SkillProposal{},err};var value protocol.SkillProposal;if err:=json.Unmarshal([]byte(body),&value);err!=nil{return protocol.SkillProposal{},err};return value,nil
}

func (database *Database) PutSkillProposalRecord(ctx context.Context,workspace string,value protocol.SkillProposal)error{body,err:=json.Marshal(value);if err!=nil{return err};_,err=database.database.ExecContext(ctx,`INSERT INTO skill_proposals(workspace_path,name,revision,body,updated_at)VALUES(?,?,?,?,?) ON CONFLICT(workspace_path,name)DO UPDATE SET revision=excluded.revision,body=excluded.body,updated_at=excluded.updated_at`,workspace,value.Name,value.Revision,string(body),encodeTime(time.Now()));return err}
func (database *Database) DeleteSkillProposalRecord(ctx context.Context,workspace,name,revision string)error{result,err:=database.database.ExecContext(ctx,`DELETE FROM skill_proposals WHERE workspace_path=? AND name=? AND revision=?`,workspace,name,revision);if err!=nil{return err};changed,_:=result.RowsAffected();if changed==0{return ErrCapabilityNotFound};return nil}

type memoryEnvelope struct { Item protocol.AgentMemoryItem `json:"item"`; Project string `json:"project,omitempty"` }
func (database *Database) ListAgentMemoryRecords(ctx context.Context,scope protocol.AgentMemoryScope,project string)([]protocol.AgentMemoryItem,error){rows,err:=database.database.QueryContext(ctx,`SELECT body FROM agent_memory WHERE json_extract(body,'$.item.scope')=? AND COALESCE(json_extract(body,'$.project'),'')=? ORDER BY updated_at DESC,id`,scope,project);if err!=nil{return nil,err};defer rows.Close();values:=make([]protocol.AgentMemoryItem,0);for rows.Next(){var body string;if err:=rows.Scan(&body);err!=nil{return nil,err};var envelope memoryEnvelope;if err:=json.Unmarshal([]byte(body),&envelope);err!=nil{return nil,err};values=append(values,envelope.Item)};return values,rows.Err()}
func (database *Database) GetAgentMemoryRecord(ctx context.Context,id string)(protocol.AgentMemoryItem,string,error){var body string;err:=database.database.QueryRowContext(ctx,`SELECT body FROM agent_memory WHERE id=?`,id).Scan(&body);if errors.Is(err,sql.ErrNoRows){return protocol.AgentMemoryItem{},"",ErrCapabilityNotFound};if err!=nil{return protocol.AgentMemoryItem{},"",err};var envelope memoryEnvelope;if err:=json.Unmarshal([]byte(body),&envelope);err!=nil{return protocol.AgentMemoryItem{},"",err};return envelope.Item,envelope.Project,nil}
func (database *Database) PutAgentMemoryRecord(ctx context.Context,item protocol.AgentMemoryItem,project string)error{body,err:=json.Marshal(memoryEnvelope{Item:item,Project:project});if err!=nil{return err};_,err=database.database.ExecContext(ctx,`INSERT INTO agent_memory(id,body,status,updated_at)VALUES(?,?,?,?) ON CONFLICT(id)DO UPDATE SET body=excluded.body,status=excluded.status,updated_at=excluded.updated_at`,item.ID,string(body),item.Status,encodeTime(item.UpdatedAt));return err}
func (database *Database) DeleteAgentMemoryRecord(ctx context.Context,id string)error{result,err:=database.database.ExecContext(ctx,`DELETE FROM agent_memory WHERE id=?`,id);if err!=nil{return err};changed,_:=result.RowsAffected();if changed==0{return ErrCapabilityNotFound};return nil}

func (database *Database) GetProjectHookTrust(ctx context.Context,project string)(bool,error){var trusted bool;err:=database.database.QueryRowContext(ctx,`SELECT trusted FROM hook_trust WHERE workspace_path=? AND hook_id='project'`,project).Scan(&trusted);if errors.Is(err,sql.ErrNoRows){return false,nil};return trusted,err}
func (database *Database) SetProjectHookTrust(ctx context.Context,project string,trusted bool)error{_,err:=database.database.ExecContext(ctx,`INSERT INTO hook_trust(workspace_path,hook_id,trusted,updated_at)VALUES(?,'project',?,?) ON CONFLICT(workspace_path,hook_id)DO UPDATE SET trusted=excluded.trusted,updated_at=excluded.updated_at`,project,trusted,encodeTime(time.Now()));return err}

func (database *Database) CreateFeedbackRecord(ctx context.Context,id string,value protocol.FeedbackRequest)error{body,err:=json.Marshal(value);if err!=nil{return err};_,err=database.database.ExecContext(ctx,`INSERT INTO feedback(id,body,created_at)VALUES(?,?,?)`,id,string(body),encodeTime(time.Now()));if err!=nil{return fmt.Errorf("sqlite: create feedback: %w",err)};return nil}
