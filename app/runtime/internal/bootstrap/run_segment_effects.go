package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/conversation"
)

// runSegmentStores adapts the composition-root stores to the run-segment effects'
// Stores port: the durable interrupt/session/transcript writes the committer
// makes, the chat-history count watermark a terminal records, and the terminal
// auto-titler. The interrupt/session/transcript stores are the same sqlite stores
// the sessions coordinator + queries coordinator hold, narrowed to the writes the
// committer needs.
type runSegmentStores struct {
	interrupts   runsegment.InterruptStore
	session      runsegment.SessionStore
	transcript   runsegment.TranscriptStore
	conversation *conversation.Messages
	titler       *maintenance.Titler
}

func (s runSegmentStores) Interrupts() runsegment.InterruptStore  { return s.interrupts }
func (s runSegmentStores) Session() runsegment.SessionStore       { return s.session }
func (s runSegmentStores) Transcript() runsegment.TranscriptStore { return s.transcript }

func (s runSegmentStores) MessageCount(ctx context.Context, sessionID string) (int, error) {
	return s.conversation.Count(ctx, sessionID)
}

func (s runSegmentStores) GenerateTitle(ctx context.Context, firstMessage string) (string, error) {
	return s.titler.Generate(ctx, firstMessage)
}

// runSegmentProcesses resolves a parked turn's persisted process id from the turn
// dispatcher — the recoverable process id the interrupt commit records.
type runSegmentProcesses struct {
	dispatcher turn.Dispatcher
}

func (p runSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.dispatcher.ProcessID(ctx, handle)
}
