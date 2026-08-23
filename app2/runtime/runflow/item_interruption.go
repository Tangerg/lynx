package runflow

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func interruptRunningItem(
	item *protocol.Item,
	problemType, detail string,
	now time.Time,
) error {
	if item == nil || item.Status != protocol.ItemStatusRunning {
		return errors.New("runflow: interruption requires a running Item")
	}
	item.Status = protocol.ItemStatusIncomplete
	switch item.Type {
	case protocol.ItemTypeToolCall:
		if item.Tool == nil {
			return errors.New("runflow: running ToolCall has no invocation")
		}
		item.FinishedAt = now.UTC()
		item.DurationMillis = nil
		item.Tool.Result = nil
		item.Error = &protocol.ProblemData{Type: problemType, Detail: detail}
	case protocol.ItemTypeAgentMessage:
		item.Phase = protocol.MessagePhaseCommentary
	case protocol.ItemTypeReasoning, protocol.ItemTypeQuestion:
	default:
		return errors.New("runflow: unsupported running Item type")
	}
	return item.ValidateWire()
}
