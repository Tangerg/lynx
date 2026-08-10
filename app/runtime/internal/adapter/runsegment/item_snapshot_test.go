package runsegment

import (
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func sameItemSnapshot(left, right transcript.Item) bool {
	return reflect.DeepEqual(normalizeItemSnapshot(left), normalizeItemSnapshot(right))
}

func normalizeItemSnapshot(item transcript.Item) transcript.ItemSnapshot {
	snapshot := item.Snapshot()
	snapshot.Identity.OccurredAt = timeFromUnixNano(snapshot.Identity.OccurredAt)
	if len(snapshot.Content) == 0 {
		snapshot.Content = nil
	}
	if snapshot.Question != nil {
		question := *snapshot.Question
		question.Fields = slices.Clone(question.Fields)
		for index := range question.Fields {
			if len(question.Fields[index].Options) == 0 {
				question.Fields[index].Options = nil
			}
		}
		if len(question.Fields) == 0 {
			question.Fields = nil
		}
		snapshot.Question = &question
	}
	return snapshot
}
