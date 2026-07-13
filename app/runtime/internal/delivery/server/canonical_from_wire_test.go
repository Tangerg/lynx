package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestCanonicalInterruptFromJSONMapPreservesQuestionOptions(t *testing.T) {
	entries := canonicalInterruptsFromWire([]protocol.Interrupt{{
		ItemID: "item_question",
		Type:   protocol.InterruptQuestion,
		Payload: map[string]any{"question": map[string]any{
			"prompt": "Choose",
			"fields": []any{map[string]any{
				"name": "answer", "label": "Answer", "type": "choice", "required": true,
				"options": []any{map[string]any{
					"label": "A", "description": "first", "preview": "alpha",
				}},
			}},
		}},
	}})
	if len(entries) != 1 || entries[0].Kind != transcript.QuestionInterrupt || entries[0].Question == nil {
		t.Fatalf("canonical interrupts = %+v", entries)
	}
	field := entries[0].Question.Fields[0]
	if field.Kind != transcript.QuestionChoice || !field.Required || len(field.Options) != 1 {
		t.Fatalf("question field = %+v", field)
	}
	if option := field.Options[0]; option.Label != "A" || option.Description != "first" || option.Preview != "alpha" {
		t.Fatalf("question option = %+v", option)
	}
}
