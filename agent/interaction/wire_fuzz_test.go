package interaction_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

type interactionProtocolValue interface {
	Validate() error
}

func FuzzSuspensionJSON(f *testing.F) {
	valid, err := json.Marshal(interaction.Suspension{
		SchemaVersion: interaction.SuspensionSchemaVersion,
		ID:            "approval-1",
		Kind:          interaction.SuspensionHuman,
		Prompt:        json.RawMessage(`{"message":"approve?"}`),
		ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
		CreatedAt:     time.Unix(1_752_568_200, 0).UTC(),
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{valid, []byte(`{}`), []byte(`{"kind":"future"}`), []byte(`null`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		assertInteractionJSONFixedPoint[interaction.Suspension](t, data)
	})
}

func FuzzInteractionEventJSON(f *testing.F) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		f.Fatal(err)
	}
	valid, err := json.Marshal(interaction.Event{Kind: interaction.EventModelRequest, Round: 1, Request: request})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{valid, []byte(`{}`), []byte(`{"kind":"future","round":1}`), []byte(`null`)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		assertInteractionJSONFixedPoint[interaction.Event](t, data)
	})
}

func assertInteractionJSONFixedPoint[T any](t *testing.T, data []byte) {
	t.Helper()
	var first T
	if err := json.Unmarshal(data, &first); err != nil {
		return
	}
	validator, ok := any(&first).(interactionProtocolValue)
	if !ok {
		t.Fatalf("%T does not implement Validate", &first)
	}
	if err := validator.Validate(); err != nil {
		t.Fatalf("successful Unmarshal produced invalid %T: %v", first, err)
	}
	firstWire, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("Marshal after successful Unmarshal: %v", err)
	}
	var second T
	if err := json.Unmarshal(firstWire, &second); err != nil {
		t.Fatalf("Unmarshal canonical %T: %v", second, err)
	}
	secondWire, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal second %T: %v", second, err)
	}
	if !bytes.Equal(firstWire, secondWire) {
		t.Fatalf("wire did not reach fixed point: first=%s second=%s", firstWire, secondWire)
	}
}
