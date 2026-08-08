package runsegment

import (
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func sameItemSnapshot(left, right transcript.Item) bool {
	return reflect.DeepEqual(normalizeItemSnapshot(left), normalizeItemSnapshot(right))
}

func normalizeItemSnapshot(item transcript.Item) transcript.Item {
	item.OccurredAt = timeFromUnixNano(item.OccurredAt)
	if len(item.Content) == 0 {
		item.Content = nil
	}
	if item.Question != nil {
		question := *item.Question
		question.Fields = slices.Clone(question.Fields)
		for index := range question.Fields {
			if len(question.Fields[index].Options) == 0 {
				question.Fields[index].Options = nil
			}
		}
		if len(question.Fields) == 0 {
			question.Fields = nil
		}
		item.Question = &question
	}
	return item
}
