package agui

import (
	"context"

	sdkevents "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// Tool helpers — fire a canned tool by id (from toolScript) and build
// the periodic telemetry payload. The DSL's RunTool() step delegates
// here; ad-hoc Tool() lives in dsl.go and assembles its events inline.

// fireTool runs a scripted tool by id (from toolScript in refactor_demo_data.go)
// against the current assistant message. Used by RunTool() in dsl.go and
// by the canned follow-up reply in reply.go.
func fireTool(ctx context.Context, send sender, parentMessageID, toolID string) bool {
	var spec *toolSpec
	for i := range toolScript {
		if toolScript[i].ID == toolID {
			spec = &toolScript[i]
			break
		}
	}
	if spec == nil {
		return true
	}
	if !send(sdkevents.NewToolCallStartEvent(
		spec.ID, spec.Fn, sdkevents.WithParentMessageID(parentMessageID),
	)) {
		return false
	}
	if !pause(ctx, 80, 220) {
		return false
	}
	if !streamToolArgs(ctx, send, spec.ID, spec.Args) {
		return false
	}
	execMin, execMax := toolExecPauseRange(spec.DurationMs)
	if !pause(ctx, execMin, execMax) {
		return false
	}
	return send(&toolCallEnd{
		ToolCallEndEvent: sdkevents.NewToolCallEndEvent(spec.ID),
		Status:           "ok",
		DurationMs:       spec.DurationMs,
		Added:            spec.Added,
		Removed:          spec.Removed,
		Hits:             spec.Hits,
		Lines:            spec.Lines,
	})
}

// toolExecPauseRange picks the "execution" pause that sits between the
// args streaming and the TOOL_CALL_END event. Scales with the tool's
// declared duration but capped so even a 2.4s typecheck doesn't stall
// the demo too long.
func toolExecPauseRange(durationMs int) (int, int) {
	execMin := 120 + durationMs/8
	execMax := execMin + 250
	if execMax > 900 {
		execMax = 900
	}
	if execMin > execMax {
		execMin = execMax - 100
	}
	return execMin, execMax
}

// telemetry builds the periodic telemetry payload — values are static
// except the activity line, which rotates.
func telemetry(activity string) map[string]any {
	return map[string]any{
		"step":       5,
		"totalSteps": 7,
		"activity":   activity,
		"tokens":     map[string]string{"used": "47.2k", "total": "200k"},
		"ctxPct":     24,
		"cost":       "0.34",
	}
}
