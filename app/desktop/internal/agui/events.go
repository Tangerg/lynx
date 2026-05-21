// Package agui implements an AG-UI mock server on top of the community
// Go SDK (github.com/ag-ui-protocol/ag-ui/sdks/community/go).
//
// All standard events come from the SDK; this file only carries the
// project-specific *extension* fields that ride along the wire (AG-UI's
// schema is open — extra JSON fields pass through validation on both ends).
//
// Two extensions:
//
//   - toolCallEnd: adds status/durationMs/added/removed/hits/lines so the
//     UI can render demo summary stats on the tool card.
//   - reasoningStart: adds parentMessageId so the UI can attach the
//     reasoning bubble to a specific assistant message.
//
// Each wrapper embeds the SDK typed event and overrides ToJSON() so the SSE
// encoder picks up the merged JSON.
package agui

import (
	"encoding/json"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// toolCallEnd extends ToolCallEndEvent with demo summary fields.
type toolCallEnd struct {
	*sdkevents.ToolCallEndEvent
	Status     string `json:"status,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`
	Added      *int   `json:"added,omitempty"`
	Removed    *int   `json:"removed,omitempty"`
	Hits       *int   `json:"hits,omitempty"`
	Lines      *int   `json:"lines,omitempty"`
}

func (e *toolCallEnd) ToJSON() ([]byte, error) { return json.Marshal(e) }

// reasoningStart extends ReasoningMessageStartEvent with our parentMessageId
// attachment hint.
type reasoningStart struct {
	*sdkevents.ReasoningMessageStartEvent
	ParentMessageID string `json:"parentMessageId,omitempty"`
}

func (e *reasoningStart) ToJSON() ([]byte, error) { return json.Marshal(e) }

// IntPtr is kept for the mock_script.go literal table.
func IntPtr(v int) *int { return &v }
