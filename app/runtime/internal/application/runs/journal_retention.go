package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// Retention accounting bounds the variable heap held by the replay window. The
// event count separately bounds fixed headers, while these conservative entry
// charges cover slice and map backing storage even when their strings are empty.
// Values are intentionally estimates: they describe retained memory, not a
// storage or delivery encoding.
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

func retainedRunBytes(run transcript.Run) int {
	bytes := retainedRunHeaderBytes + stringsBytes(
		run.SessionID,
		run.ID,
		run.SpawnedByItemID,
		run.ParentRunID,
		run.RootRunID,
		run.GoalLeaseID,
		run.ActiveSegmentID,
		run.Detail,
		run.ModelSelection.Provider(),
		run.ModelSelection.Model(),
	)
	bytes += retainedProblemBytes(run.Error)
	bytes += retainedUsageBytes(run.Metrics.Usage)
	bytes += cap(run.Capabilities.InterruptKinds) * retainedSliceEntryBytes
	bytes += cap(run.Interrupts) * retainedInterruptBytes
	for _, interrupt := range run.Interrupts {
		bytes += retainedInterruptPayloadBytes(interrupt)
	}
	return bytes
}

func retainedItemBytes(item transcript.Item) int {
	bytes := retainedItemHeaderBytes + stringsBytes(
		item.SessionID,
		item.ID,
		item.RunID,
		item.Text,
		item.Summary,
		string(item.SafetyClass),
	)
	bytes += cap(item.Content) * retainedSliceEntryBytes
	for _, block := range item.Content {
		bytes += len(block.Text) + len(block.MediaType) + cap(block.Bytes)
	}
	bytes += retainedQuestionBytes(item.Question)
	bytes += retainedToolInvocationBytes(item.Tool)
	bytes += retainedProblemBytes(item.Error)
	return bytes
}

func retainedStateSnapshotBytes(snapshot StateSnapshot) int {
	bytes := retainedEventHeaderBytes + len(snapshot.SessionID) + cap(snapshot.Plan)*retainedPlanEntryBytes
	for _, item := range snapshot.Plan {
		bytes += len(item.ID) + len(item.Description)
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

func retainedProblemBytes(problem *transcript.Problem) int {
	if problem == nil {
		return 0
	}
	return len(problem.Detail) + len(problem.DocURL)
}

func retainedUsageBytes(usage *transcript.Usage) int {
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
