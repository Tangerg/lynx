package server

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// sessionReadUseCases is the session-scoped read residue the delivery layer
// still reads off the Runtime facade: the durable transcript projection (item/run
// history) and the chat-history projection. The session-aggregate CRUD and
// lifecycle write-sets live on the sessions coordinator (see [Server.sessions]);
// these projections move to their own coordinators in a later batch. Individual
// handler methods still call only the operation they translate to wire.
type sessionReadUseCases interface {
	ListTranscript(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
	ListTranscriptRuns(ctx context.Context, sessionID string) ([]transcript.Run, error)
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
}
