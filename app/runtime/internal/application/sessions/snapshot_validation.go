package sessions

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ValidateToolResults verifies that every typed transcript offload has exactly
// one matching portable blob and that no blob is detached from its item. A
// coherent read snapshot carries the hydrated body in Tool.Result, while a
// restore projection carries the inline preview; both representations are
// valid as long as the typed relationship and content agree with the blob.
func (snapshot Snapshot) ValidateToolResults() error {
	byItem := make(map[string]toolresult.Blob, len(snapshot.ToolResults))
	byID := make(map[toolresult.ID]string, len(snapshot.ToolResults))
	for index, blob := range snapshot.ToolResults {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("sessions: tool result %d: %w", index, err)
		}
		if blob.SessionID != snapshot.Session.ID() {
			return fmt.Errorf("sessions: tool result %q belongs to session %q, want %q", blob.ID, blob.SessionID, snapshot.Session.ID())
		}
		if _, duplicate := byItem[blob.ItemID]; duplicate {
			return fmt.Errorf("sessions: multiple tool results are bound to item %q", blob.ItemID)
		}
		if owner, duplicate := byID[blob.ID]; duplicate {
			return fmt.Errorf("sessions: tool result %q is bound to both items %q and %q", blob.ID, owner, blob.ItemID)
		}
		byItem[blob.ItemID] = blob
		byID[blob.ID] = blob.ItemID
	}

	for _, item := range snapshot.Items {
		invocation, present := item.ToolInvocation()
		if !present || invocation.Offload == nil {
			continue
		}
		ref := *invocation.Offload
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("sessions: item %q offload: %w", item.ID(), err)
		}
		if invocation.Result == nil {
			return fmt.Errorf("sessions: item %q offloaded result is absent", item.ID())
		}
		result, ok := invocation.Result.String()
		if !ok {
			return fmt.Errorf("sessions: item %q offloaded result is not a string", item.ID())
		}
		blob, exists := byItem[item.ID()]
		if !exists {
			return fmt.Errorf("sessions: item %q references missing tool result %q", item.ID(), ref.ID)
		}
		if blob.ID != ref.ID || blob.ToolName != invocation.Name {
			return fmt.Errorf("sessions: item %q and tool result %q disagree on identity or tool", item.ID(), blob.ID)
		}
		if result != blob.Preview && result != blob.Body {
			return fmt.Errorf("sessions: item %q result matches neither tool result %q preview nor body", item.ID(), blob.ID)
		}
		delete(byItem, item.ID())
	}
	for itemID, blob := range byItem {
		return fmt.Errorf("sessions: tool result %q references missing transcript item %q", blob.ID, itemID)
	}
	return nil
}

// NormalizeForRestore returns a copy whose offloaded transcript results use
// their bounded previews. This is the only representation written back to
// history: full bodies remain in ToolResults and are joined structurally on
// reads. The source snapshot is not mutated.
func (snapshot Snapshot) NormalizeForRestore() (Snapshot, error) {
	if err := snapshot.ValidateToolResults(); err != nil {
		return Snapshot{}, err
	}
	if len(snapshot.ToolResults) == 0 {
		return snapshot, nil
	}

	byItem := make(map[string]toolresult.Blob, len(snapshot.ToolResults))
	for _, blob := range snapshot.ToolResults {
		byItem[blob.ItemID] = blob
	}

	normalized := snapshot
	normalized.Items = append([]transcript.Item(nil), snapshot.Items...)
	for i := range normalized.Items {
		item := &normalized.Items[i]
		itemSnapshot := item.Snapshot()
		if itemSnapshot.Tool == nil || itemSnapshot.Tool.Offload == nil {
			continue
		}
		blob := byItem[item.ID()]
		preview := tool.StringResult(blob.Preview)
		itemSnapshot.Tool.Result = &preview
		itemSnapshot.Tool.Offload = &toolresult.Ref{ID: blob.ID}
		restored, err := transcript.RestoreItem(itemSnapshot)
		if err != nil {
			return Snapshot{}, fmt.Errorf("sessions: normalize item %q: %w", item.ID(), err)
		}
		*item = restored
	}
	return normalized, nil
}
