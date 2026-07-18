package server

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func canonicalToolResults(art protocol.SessionArtifact, items []transcript.Item) ([]offload.ToolResultBlob, error) {
	itemsByID := make(map[string]*transcript.Item, len(items))
	for i := range items {
		itemsByID[items[i].ID] = &items[i]
	}
	seenIDs := make(map[offload.ID]struct{}, len(art.ToolResults))
	seenItems := make(map[string]struct{}, len(art.ToolResults))
	blobs := make([]offload.ToolResultBlob, 0, len(art.ToolResults))
	for i, encoded := range art.ToolResults {
		path := fmt.Sprintf("artifact.toolResults[%d]", i)
		id, err := offload.ParseID(encoded.ID)
		if err != nil {
			return nil, invalidArtifact(path+".id", "%v", err)
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return nil, invalidArtifact(path+".id", "duplicate id %q", id)
		}
		if _, duplicate := seenItems[encoded.ItemID]; encoded.ItemID != "" && duplicate {
			return nil, invalidArtifact(path+".itemId", "duplicate item binding %q", encoded.ItemID)
		}
		item := itemsByID[encoded.ItemID]
		if item == nil {
			return nil, invalidArtifact(path+".itemId", "references unknown item %q", encoded.ItemID)
		}
		if item.Tool == nil {
			return nil, invalidArtifact(path+".itemId", "must reference a toolCall item")
		}
		if item.Status != transcript.ItemCompleted || item.Error != nil {
			return nil, invalidArtifact(path+".itemId", "must reference a successfully completed toolCall item")
		}
		if encoded.ToolName != item.Tool.Name {
			return nil, invalidArtifact(path+".toolName", "got %q, want item tool %q", encoded.ToolName, item.Tool.Name)
		}
		itemPreview, ok := item.Tool.Result.(string)
		if !ok || itemPreview != encoded.Preview {
			return nil, invalidArtifact(path+".preview", "must equal the bound item's string result")
		}
		blob := offload.ToolResultBlob{
			ID: id, SessionID: art.Session.ID, ItemID: encoded.ItemID,
			ToolName: encoded.ToolName, Preview: encoded.Preview, Body: encoded.Body,
			CreatedAt: encoded.CreatedAt,
		}
		if err := blob.Validate(); err != nil {
			return nil, invalidArtifact(path, "%v", err)
		}
		item.Tool.Offload = &offload.Ref{ID: id}
		seenIDs[id] = struct{}{}
		seenItems[encoded.ItemID] = struct{}{}
		blobs = append(blobs, blob)
	}
	return blobs, nil
}

func canonicalArtifact(art protocol.SessionArtifact, messageCount int) ([]transcript.Run, []transcript.Item, error) {
	runs := make([]transcript.Run, 0, len(art.Runs))
	runIDs := make(map[string]struct{}, len(art.Runs))
	runsByID := make(map[string]transcript.Run, len(art.Runs))
	for i, entry := range art.Runs {
		path := fmt.Sprintf("artifact.runs[%d]", i)
		if _, duplicate := runIDs[entry.Run.ID]; entry.Run.ID != "" && duplicate {
			return nil, nil, invalidArtifact(path+".run.id", "duplicate id %q", entry.Run.ID)
		}
		if entry.MessageMark < 0 || entry.MessageMark > messageCount {
			return nil, nil, invalidArtifact(path+".messageMark", "must be between 0 and %d", messageCount)
		}
		run, err := canonicalRunFromWire(art.Session.ID, path+".run", entry.Run, entry.UpdatedAt, entry.MessageMark)
		if err != nil {
			return nil, nil, err
		}
		runIDs[run.ID] = struct{}{}
		runsByID[run.ID] = run
		runs = append(runs, run)
	}

	items := make([]transcript.Item, 0, len(art.Items))
	itemIDs := make(map[string]transcript.Item, len(art.Items))
	for i, entry := range art.Items {
		path := fmt.Sprintf("artifact.items[%d].item", i)
		if _, duplicate := itemIDs[entry.Item.ID]; entry.Item.ID != "" && duplicate {
			return nil, nil, invalidArtifact(path+".id", "duplicate id %q", entry.Item.ID)
		}
		item, err := canonicalItemFromWire(art.Session.ID, path, entry.Item)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := runIDs[item.RunID]; !ok {
			return nil, nil, invalidArtifact(path+".runId", "references unknown run %q", item.RunID)
		}
		itemIDs[item.ID] = item
		items = append(items, item)
	}

	for i, run := range runs {
		path := fmt.Sprintf("artifact.runs[%d].run", i)
		if run.SpawnedByItemID != "" {
			item, ok := itemIDs[run.SpawnedByItemID]
			if !ok {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "references unknown item %q", run.SpawnedByItemID)
			}
			if item.Kind != transcript.ToolCall {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "must reference a toolCall item")
			}
			if item.RunID == run.ID {
				return nil, nil, invalidArtifact(path+".spawnedByItemId", "cannot reference an item from the spawned run itself")
			}
		}
	}
	if err := validateRunTree(runs, itemIDs); err != nil {
		return nil, nil, err
	}
	for i, item := range items {
		if runsByID[item.RunID].State.IsTerminal() && item.Status == transcript.ItemRunning {
			return nil, nil, invalidArtifact(
				fmt.Sprintf("artifact.items[%d].item.status", i),
				"cannot be running after its run has terminated",
			)
		}
	}
	return runs, items, nil
}

func validateRunTree(runs []transcript.Run, items map[string]transcript.Item) error {
	parents := make(map[string]string, len(runs))
	paths := make(map[string]string, len(runs))
	for i, run := range runs {
		if run.SpawnedByItemID == "" {
			continue
		}
		parents[run.ID] = items[run.SpawnedByItemID].RunID
		paths[run.ID] = fmt.Sprintf("artifact.runs[%d].run.spawnedByItemId", i)
	}
	states := make(map[string]uint8, len(parents))
	for origin := range parents {
		if states[origin] == 2 {
			continue
		}
		var path []string
		for current := origin; current != ""; current = parents[current] {
			if states[current] == 1 {
				return invalidArtifact(paths[origin], "creates a cycle in the run tree")
			}
			if states[current] == 2 {
				break
			}
			states[current] = 1
			path = append(path, current)
		}
		for _, runID := range path {
			states[runID] = 2
		}
	}
	return nil
}

func invalidArtifact(path, format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s: %s", protocol.ErrInvalidParams, path, detail)
}
