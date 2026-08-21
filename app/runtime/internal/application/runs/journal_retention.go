package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Retention accounting bounds the variable heap held by the replay window. The
// event count separately bounds fixed headers, while these conservative entry
// charges cover slice and map backing storage even when their strings are empty.
// Values are intentionally estimates: they describe retained memory, not any
// durable or presentation encoding.
const (
	retainedEventHeaderBytes    = 128
	retainedRunHeaderBytes      = 256
	retainedItemHeaderBytes     = 192
	retainedSliceEntryBytes     = 64
	retainedMapEntryBytes       = 96
	retainedQuestionFieldBytes  = 96
	retainedQuestionOptionBytes = 64
	retainedInterruptBytes      = 128
	retainedPlanEntryBytes      = 64
)

// retainedBytes is charged exactly once, when the event is published. Eviction
// and replay reuse the charge stored beside the event.
func retainedBytes(event Event) int {
	if event.Payload == nil {
		return retainedEventHeaderBytes + len(event.RunID) + len(event.SegmentID) + len(event.Cursor)
	}
	return retainedEventHeaderBytes + len(event.RunID) + len(event.SegmentID) + len(event.Cursor) +
		event.Payload.retainedBytes()
}

func retainedRunBytes(run run.Run) int {
	snapshot := run.Snapshot()
	bytes := retainedRunHeaderBytes + stringsBytes(
		snapshot.SessionID,
		snapshot.ID,
		snapshot.Lineage.SpawnedByItemID,
		snapshot.Lineage.ParentRunID,
		snapshot.Lineage.RootRunID,
		snapshot.GoalIncarnationID,
		snapshot.ActiveSegmentID,
		snapshot.Detail,
		snapshot.ModelSelection.Provider(),
		snapshot.ModelSelection.Model(),
	)
	if snapshot.Failure != nil {
		bytes += len(snapshot.Failure.Detail) + len(snapshot.Failure.DocURL)
	}
	usage, _ := snapshot.Metrics.Usage()
	bytes += retainedUsageBytes(&usage)
	bytes += cap(snapshot.Capabilities.InterruptKinds) * retainedSliceEntryBytes
	return bytes
}

func retainedItemBytes(item transcript.Item) int {
	bytes := retainedItemHeaderBytes + stringsBytes(
		item.SessionID(),
		item.ID(),
		item.RunID(),
		item.Text(),
		item.Summary(),
		string(item.SafetyClass()),
	)
	content := item.Content()
	bytes += cap(content) * retainedSliceEntryBytes
	for _, block := range content {
		bytes += len(block.Text) + len(block.MediaType) + cap(block.Bytes)
	}
	if question, ok := item.Question(); ok {
		bytes += retainedQuestionBytes(&question)
	}
	if invocation, ok := item.ToolInvocation(); ok {
		bytes += retainedToolInvocationBytes(&invocation)
	}
	if failure, ok := item.Failure(); ok {
		bytes += retainedToolFailureBytes(&failure)
	}
	return bytes
}

func retainedItemStartBytes(item ItemStart) int {
	bytes := retainedItemHeaderBytes + stringsBytes(
		item.SessionID,
		item.RunID,
		item.ItemID,
		string(item.SafetyClass),
	)
	bytes += retainedToolInvocationBytes(item.ToolInvocation)
	return bytes
}

func retainedPlanSnapshotBytes(snapshot PlanSnapshot) int {
	bytes := retainedEventHeaderBytes + len(snapshot.SessionID) + cap(snapshot.Steps)*retainedPlanEntryBytes
	for _, item := range snapshot.Steps {
		bytes += len(item.Description)
	}
	return bytes
}

func retainedInterruptPayloadBytes(interrupt transcript.Interrupt) int {
	bytes := len(interrupt.ItemID) + len(interrupt.RunID)
	if interrupt.Approval != nil {
		bytes += retainedToolInvocationBytes(&interrupt.Approval.Tool)
		bytes += len(interrupt.Approval.Risk) + len(interrupt.Approval.Reason)
	}
	bytes += retainedQuestionBytes(interrupt.Question)
	return bytes
}

func retainedQuestionBytes(question *transcript.Question) int {
	if question == nil {
		return 0
	}
	bytes := cap(question.Fields) * retainedQuestionFieldBytes
	for _, field := range question.Fields {
		bytes += stringsBytes(field.Prompt, field.Header)
		bytes += cap(field.Options) * retainedQuestionOptionBytes
		for _, option := range field.Options {
			bytes += stringsBytes(option.Label, option.Description, option.Preview)
		}
	}
	return bytes
}

func retainedToolInvocationBytes(invocation *transcript.ToolInvocation) int {
	if invocation == nil {
		return 0
	}
	bytes := len(invocation.Name) + len(invocation.Arguments.Canonical())
	if invocation.Result != nil {
		bytes += len(invocation.Result.Canonical())
	}
	if invocation.Offload != nil {
		bytes += len(invocation.Offload.ID.String())
	}
	return bytes
}

func retainedToolFailureBytes(failure *tool.Failure) int {
	if failure == nil {
		return 0
	}
	return len(failure.Detail) + len(failure.DocURL)
}

func retainedUsageBytes(usage *accounting.Usage) int {
	if usage == nil {
		return 0
	}
	bytes := len(usage.ByModel) * retainedMapEntryBytes
	for model := range usage.ByModel {
		bytes += len(model)
	}
	return bytes
}

func stringsBytes(values ...string) int {
	var total int
	for _, value := range values {
		total += len(value)
	}
	return total
}
