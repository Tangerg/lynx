package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/compactionflow"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func(database *Database)CompactionCandidate(ctx context.Context,sessionID,runID,workspace string)(compactionflow.Candidate,error){
	transaction,err:=database.database.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return compactionflow.Candidate{},err};defer transaction.Rollback()
	var open bool;if err:=transaction.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM runs WHERE session_id=? AND parent_run_id IS NULL AND status!='finished')`,sessionID).Scan(&open);err!=nil{return compactionflow.Candidate{},err};if open{if err:=transaction.Commit();err!=nil{return compactionflow.Candidate{},err};return compactionflow.Candidate{},nil}
	var provider,model string;if err:=transaction.QueryRowContext(ctx,`SELECT provider,model FROM runs WHERE id=? AND session_id=? AND status='finished'`,runID,sessionID).Scan(&provider,&model);err!=nil{return compactionflow.Candidate{},err}
	through:=-1;var summary []byte;err=transaction.QueryRowContext(ctx,`SELECT through_ordinal,summary_body FROM conversation_compactions WHERE session_id=? ORDER BY through_ordinal DESC LIMIT 1`,sessionID).Scan(&through,&summary);if err!=nil&&!errors.Is(err,sql.ErrNoRows){return compactionflow.Candidate{},err}
	result:=compactionflow.Candidate{SessionID:sessionID,RunID:runID,Workspace:workspace,Provider:provider,Model:model,LatestThrough:through,MaximumOrdinal:-1}
	if len(summary)>0{result.Entries=append(result.Entries,compactionflow.Entry{Ordinal:through,Body:summary})}
	rows,err:=transaction.QueryContext(ctx,`SELECT ordinal,body FROM conversation_messages WHERE session_id=? AND ordinal>? ORDER BY ordinal`,sessionID,through);if err!=nil{return compactionflow.Candidate{},err};defer rows.Close();for rows.Next(){var entry compactionflow.Entry;if err:=rows.Scan(&entry.Ordinal,&entry.Body);err!=nil{return compactionflow.Candidate{},err};result.Entries=append(result.Entries,entry);result.MaximumOrdinal=entry.Ordinal};if err:=rows.Err();err!=nil{return compactionflow.Candidate{},err};if err:=transaction.Commit();err!=nil{return compactionflow.Candidate{},err};return result,nil
}

func(database *Database)CompactionRecoveries(ctx context.Context)([]compactionflow.Recovery,error){
	rows,err:=database.database.QueryContext(ctx,`
		SELECT sessions.id,runs.id,sessions.workspace_path
		FROM sessions
		JOIN runs ON runs.id=(
			SELECT latest.id FROM runs AS latest
			WHERE latest.session_id=sessions.id AND latest.parent_run_id IS NULL AND latest.status='finished'
			ORDER BY latest.finished_at DESC,latest.id DESC LIMIT 1
		)
		WHERE EXISTS(SELECT 1 FROM conversation_messages WHERE conversation_messages.session_id=sessions.id)
		AND NOT EXISTS(SELECT 1 FROM runs AS open WHERE open.session_id=sessions.id AND open.parent_run_id IS NULL AND open.status!='finished')
		ORDER BY sessions.id`)
	if err!=nil{return nil,err};defer rows.Close();values:=make([]compactionflow.Recovery,0)
	for rows.Next(){var value compactionflow.Recovery;if err:=rows.Scan(&value.SessionID,&value.RunID,&value.Workspace);err!=nil{return nil,err};values=append(values,value)}
	if err:=rows.Err();err!=nil{return nil,err};return values,nil
}

func(database *Database)CommitCompaction(ctx context.Context,write compactionflow.Write)(bool,error){
	if write.ID==""||write.SessionID==""||write.RunID==""||write.ThroughOrdinal<0||write.MessagesBefore<=write.MessagesAfter||!json.Valid(write.SummaryBody){return false,errors.New("sqlite: invalid compaction write")}
	transaction,err:=database.database.BeginTx(ctx,nil);if err!=nil{return false,err};defer transaction.Rollback();latest:=-1;err=transaction.QueryRowContext(ctx,`SELECT through_ordinal FROM conversation_compactions WHERE session_id=? ORDER BY through_ordinal DESC LIMIT 1`,write.SessionID).Scan(&latest);if err!=nil&&!errors.Is(err,sql.ErrNoRows){return false,err};if latest!=write.ExpectedLatestThrough{return false,nil}
	var maximum int;if err:=transaction.QueryRowContext(ctx,`SELECT coalesce(max(ordinal),-1) FROM conversation_messages WHERE session_id=?`,write.SessionID).Scan(&maximum);err!=nil{return false,err};if maximum!=write.ExpectedMaximumOrdinal{return false,nil}
	if _,err:=transaction.ExecContext(ctx,`INSERT INTO conversation_compactions(id,session_id,run_id,through_ordinal,summary_body,messages_before,messages_after,created_at) VALUES(?,?,?,?,?,?,?,?)`,write.ID,write.SessionID,write.RunID,write.ThroughOrdinal,string(write.SummaryBody),write.MessagesBefore,write.MessagesAfter,encodeTime(write.CreatedAt));err!=nil{return false,fmt.Errorf("sqlite: record compaction: %w",err)}
	var ordinal int;if err:=transaction.QueryRowContext(ctx,`SELECT coalesce(max(ordinal),-1)+1 FROM items WHERE run_id=?`,write.RunID).Scan(&ordinal);err!=nil{return false,err}
	item:=protocol.Item{ID:write.ID,RunID:write.RunID,Status:protocol.ItemStatusCompleted,CreatedAt:write.CreatedAt,Type:protocol.ItemTypeCompaction,DroppedMessages:write.MessagesBefore-write.MessagesAfter};body,err:=json.Marshal(item);if err!=nil{return false,err};record:=transcript.Record{ID:write.ID,SessionID:write.SessionID,RunID:write.RunID,Ordinal:ordinal,Body:body,SearchText:"",CreatedAt:write.CreatedAt};if err:=insertItem(ctx,transaction,record);err!=nil{return false,err};if err:=transaction.Commit();err!=nil{return false,err};return true,nil
}
