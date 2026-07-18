package sessions

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ValidateToolResults verifies that every typed transcript offload has exactly
// one matching portable blob and that no blob is detached from its item. A
// coherent read snapshot carries the hydrated body in Tool.Result, while a
// restore projection carries the inline preview; both representations are
// valid as long as the typed relationship and content agree with the blob.
func (s Snapshot) ValidateToolResults() error {
	byItem := make(map[string]offload.ToolResultBlob, len(s.ToolResults))
	byID := make(map[offload.ID]string, len(s.ToolResults))
	for index, blob := range s.ToolResults {
		if err := blob.Validate(); err != nil {
			return fmt.Errorf("sessions: tool result %d: %w", index, err)
		}
		if blob.SessionID != s.Session.ID {
			return fmt.Errorf("sessions: tool result %q belongs to session %q, want %q", blob.ID, blob.SessionID, s.Session.ID)
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

	for _, item := range s.Items {
		if item.Tool == nil || item.Tool.Offload == nil {
			continue
		}
		ref := *item.Tool.Offload
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("sessions: item %q offload: %w", item.ID, err)
		}
		if item.Tool.Result == nil {
			return fmt.Errorf("sessions: item %q offloaded result is absent", item.ID)
		}
		result, ok := item.Tool.Result.String()
		if !ok {
			return fmt.Errorf("sessions: item %q offloaded result is not a string", item.ID)
		}
		blob, exists := byItem[item.ID]
		if !exists {
			return fmt.Errorf("sessions: item %q references missing tool result %q", item.ID, ref.ID)
		}
		if blob.ID != ref.ID || blob.ToolName != item.Tool.Name {
			return fmt.Errorf("sessions: item %q and tool result %q disagree on identity or tool", item.ID, blob.ID)
		}
		if result != blob.Preview && result != blob.Body {
			return fmt.Errorf("sessions: item %q result matches neither tool result %q preview nor body", item.ID, blob.ID)
		}
		delete(byItem, item.ID)
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
func (s Snapshot) NormalizeForRestore() (Snapshot, error) {
	if err := s.ValidateToolResults(); err != nil {
		return Snapshot{}, err
	}
	if len(s.ToolResults) == 0 {
		return s, nil
	}

	byItem := make(map[string]offload.ToolResultBlob, len(s.ToolResults))
	for _, blob := range s.ToolResults {
		byItem[blob.ItemID] = blob
	}

	normalized := s
	normalized.Items = append([]transcript.Item(nil), s.Items...)
	for i := range normalized.Items {
		item := &normalized.Items[i]
		if item.Tool == nil || item.Tool.Offload == nil {
			continue
		}
		blob := byItem[item.ID]
		invocation := *item.Tool
		preview := tool.StringResult(blob.Preview)
		invocation.Result = &preview
		invocation.Offload = &offload.Ref{ID: blob.ID}
		item.Tool = &invocation
	}
	return normalized, nil
}
