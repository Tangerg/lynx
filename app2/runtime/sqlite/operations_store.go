package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/accounting"
)

func (database *Database) ListUsageRunRecords(ctx context.Context, sessionID string, since time.Time) ([]accounting.RunRecord,error){query:=`SELECT session_id,provider,model,body,finished_at FROM runs WHERE status='finished'`;args:=make([]any,0,2);if sessionID!=""{query+=` AND session_id=?`;args=append(args,sessionID)};if !since.IsZero(){query+=` AND finished_at>=?`;args=append(args,encodeTime(since))};query+=` ORDER BY finished_at,id`;rows,err:=database.database.QueryContext(ctx,query,args...);if err!=nil{return nil,err};defer rows.Close();values:=make([]accounting.RunRecord,0);for rows.Next(){var value accounting.RunRecord;var body,finished string;if err:=rows.Scan(&value.SessionID,&value.Provider,&value.Model,&body,&finished);err!=nil{return nil,err};value.Body=[]byte(body);value.FinishedAt,err=decodeTime(finished);if err!=nil{return nil,err};values=append(values,value)};return values,rows.Err()}
func (database *Database) SessionExists(ctx context.Context,id string)(bool,error){var one int;err:=database.database.QueryRowContext(ctx,`SELECT 1 FROM sessions WHERE id=?`,id).Scan(&one);if errors.Is(err,sql.ErrNoRows){return false,nil};return err==nil,err}
