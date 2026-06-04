package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/history"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// persistStreamEvent records the durable side of one stream event to the
// Item-history store as a run streams (API.md §7.4 / §10.3): completed
// Items are appended; the run's RunRef is upserted on start (running) and
// finish (finished + outcome). Best-effort + a no-op when no history store
// is configured — items.list then falls back to message reconstruction.
func (i *Server) persistStreamEvent(ctx context.Context, runID, sessionID, parentRunID string, se protocol.StreamEvent) {
	if i.rt.History() == nil {
		return
	}
	switch se.Type {
	case protocol.StreamItemCompleted:
		i.persistItem(ctx, sessionID, se.Item)
	case protocol.StreamRunStarted:
		i.persistRun(ctx, sessionID, se.Run)
	case protocol.StreamRunFinished:
		// run.finished carries only the outcome; synthesize the terminal
		// RunRef so history records the run's final status + outcome.
		i.persistRun(ctx, sessionID, &protocol.RunRef{
			ID:          runID,
			SessionID:   sessionID,
			ParentRunID: parentRunID,
			Status:      protocol.RunStatusFinished,
			Outcome:     se.Outcome,
			FinishedAt:  time.Now().UTC(),
		})
	}
}

// persistItem appends one completed Item to the history store.
func (i *Server) persistItem(ctx context.Context, sessionID string, item *protocol.Item) {
	store := i.rt.History()
	if store == nil || item == nil {
		return
	}
	blob, err := json.Marshal(item)
	if err != nil {
		return
	}
	_ = store.AppendItem(ctx, history.Item{
		SessionID: sessionID,
		RunID:     item.RunID,
		ItemID:    item.ID,
		CreatedAt: item.CreatedAt,
		Blob:      blob,
	})
}

// persistRun upserts one RunRef into the history store.
func (i *Server) persistRun(ctx context.Context, sessionID string, run *protocol.RunRef) {
	store := i.rt.History()
	if store == nil || run == nil {
		return
	}
	blob, err := json.Marshal(run)
	if err != nil {
		return
	}
	_ = store.PutRun(ctx, history.Run{
		SessionID: sessionID,
		RunID:     run.ID,
		UpdatedAt: time.Now().UTC(),
		Blob:      blob,
	})
}

// persistUserItem records the user's input as a completed userMessage
// Item at run start. The translator never emits one (the client already
// holds the input it sent), but durable history must include the user
// turn (API.md §0.1 / §7.4). The id is derived from the runId so it stays
// stable and distinct from the run's streamed items.
func (i *Server) persistUserItem(ctx context.Context, sessionID, runID string, input []protocol.ContentBlock) {
	if i.rt.History() == nil || len(input) == 0 {
		return
	}
	i.persistItem(ctx, sessionID, &protocol.Item{
		ID:        protocol.IDPrefixItem + runID + "_u",
		RunID:     runID,
		Status:    protocol.ItemStatusCompleted,
		Type:      protocol.ItemTypeUserMessage,
		CreatedAt: time.Now().UTC(),
		Content:   input,
	})
}
