package interactioninput

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func FuzzContinuationCodec(f *testing.F) {
	prompt := json.RawMessage(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`)
	valid, err := json.Marshal(continuationWire{
		SchemaVersion: continuationSchemaVersion,
		Key:           "approval.shell",
		PromptDigest:  promptDigest(prompt),
		Prompt:        prompt,
	})
	if err != nil {
		f.Fatalf("encode continuation seed: %v", err)
	}
	f.Add(valid)
	for _, seed := range [][]byte{
		[]byte(`{"SchemaVersion":1}`),
		[]byte(`{"schema_version":1,"schema_version":2}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var continuation continuationWire
		if err := decode(raw, &continuation); err != nil {
			return
		}
		if continuation.SchemaVersion != continuationSchemaVersion || continuation.Key == "" || continuation.Prompt == nil {
			return
		}
		prompt, err := DecodePrompt(continuation.Prompt)
		if err != nil {
			return
		}
		canonicalPrompt, err := EncodePrompt(prompt)
		if err != nil || !bytes.Equal(canonicalPrompt, continuation.Prompt) || continuation.PromptDigest != promptDigest(canonicalPrompt) {
			return
		}
		encoded, err := json.Marshal(continuation)
		if err != nil {
			t.Fatalf("encode decoded continuation: %v", err)
		}
		var roundTripped continuationWire
		if err := decode(encoded, &roundTripped); err != nil {
			t.Fatalf("decode re-encoded continuation: %v", err)
		}
		if !reflect.DeepEqual(roundTripped, continuation) {
			t.Fatalf("continuation changed across round trip: got %#v, want %#v", roundTripped, continuation)
		}
	})
}

func FuzzPromptCodec(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","fields":[{"prompt":"Continue?","allowCustom":true}]}}`),
		[]byte(`{"kind":"future"}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		prompt, err := DecodePrompt(raw)
		if err != nil {
			return
		}
		encoded, err := EncodePrompt(prompt)
		if err != nil {
			t.Fatalf("encode decoded prompt: %v", err)
		}
		roundTripped, err := DecodePrompt(encoded)
		if err != nil {
			t.Fatalf("decode re-encoded prompt: %v", err)
		}
		if !reflect.DeepEqual(roundTripped, prompt) {
			t.Fatalf("prompt changed across round trip: got %#v, want %#v", roundTripped, prompt)
		}
	})
}

func FuzzResolutionCodec(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"approved":true,"arguments":"{}","remember_scope":"session"}`),
		[]byte(`{"approved":false,"reason":"not now"}`),
		[]byte(`{"answers":[["yes"]]}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		resolution, err := DecodeResolution(raw)
		if err != nil {
			return
		}
		encoded, err := EncodeResolution(resolution)
		if err != nil {
			t.Fatalf("encode decoded resolution: %v", err)
		}
		roundTripped, err := DecodeResolution(encoded)
		if err != nil {
			t.Fatalf("decode re-encoded resolution: %v", err)
		}
		if !reflect.DeepEqual(roundTripped, resolution) {
			t.Fatalf("resolution changed across round trip: got %#v, want %#v", roundTripped, resolution)
		}
	})
}
