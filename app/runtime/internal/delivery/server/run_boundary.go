package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// sessions.rollback truncates a session's history at a run boundary in place;
// sessions.fork{fromRunId} truncate-copies it into a child. Both reason over
// the per-run message watermark (transcript.Run.Mark, recorded at run.finished
// — see transcript.go) to map a run boundary onto a chat-memory message count,
// since the message log itself carries no run markers.

// runNodes lifts the structured timeline fields out of each persisted run's
// opaque wire blob (a marshaled [protocol.RunRef]) so the domain boundary math
// ([transcript.BoundaryAt]) stays wire-free. It also returns a by-id index of
// the original RunRefs, because the rollback response reports dropped runs as
// full wire RunRefs.
func runNodes(runs []transcript.Run) ([]transcript.RunNode, map[string]protocol.RunRef, error) {
	nodes := make([]transcript.RunNode, 0, len(runs))
	byID := make(map[string]protocol.RunRef, len(runs))
	for _, r := range runs {
		var ref protocol.RunRef
		if err := json.Unmarshal(r.Blob, &ref); err != nil {
			return nil, nil, fmt.Errorf("server: decode run %q: %w", r.RunID, err)
		}
		nodes = append(nodes, transcript.RunNode{
			ID:              ref.ID,
			ParentRunID:     ref.ParentRunID,
			SpawnedByItemID: ref.SpawnedByItemID,
			CreatedAt:       ref.CreatedAt,
			Mark:            r.Mark,
		})
		byID[ref.ID] = ref
	}
	return nodes, byID, nil
}

// wireBoundaryErr maps the transcript boundary sentinels onto their wire errors
// (the domain layer is protocol-free; the adapter owns the wire mapping).
func wireBoundaryErr(err error) error {
	switch {
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	default:
		return err
	}
}

// openingUserInput maps each run id to the content of its FIRST userMessage
// item — the opening turn the client re-populates the composer from. Runs with
// no opening user turn (resume / edit continuations) are absent from the map.
func openingUserInput(items []transcript.Item) map[string][]protocol.ContentBlock {
	out := map[string][]protocol.ContentBlock{}
	for _, it := range items {
		if _, seen := out[it.RunID]; seen {
			continue
		}
		var item protocol.Item
		if err := json.Unmarshal(it.Blob, &item); err != nil {
			continue
		}
		if item.Type != protocol.ItemTypeUserMessage {
			continue
		}
		out[it.RunID] = item.Content
	}
	return out
}
